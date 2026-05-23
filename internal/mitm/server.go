package mitm

import (
	"bufio"
	"bytes"
	"context"
	"crypto/tls"
	"errors"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"megiddo/internal/assetbatch"
	"megiddo/internal/localserve"
	"megiddo/internal/megiddo"
	"megiddo/internal/replacement"
	"megiddo/internal/texpacklookup"
)

type Server struct {
	UpstreamIPs   map[string][]string // keys must already be lowercase hostnames
	Leaves        map[string]*tls.Certificate
	DefaultLeaf   *tls.Certificate
	Logf          func(format string, args ...any)
	Replacements  *replacement.Map
	LocalFiles    *localserve.Store
	TexpackLookup *texpacklookup.Store

	listener net.Listener
}

func (s *Server) logger() func(string, ...any) {
	if s.Logf != nil {
		return s.Logf
	}
	return func(format string, args ...any) { log.Printf(format, args...) }
}

func (s *Server) Listen(bindAddr string) error {
	conf := &tls.Config{
		MinVersion: tls.VersionTLS12,
		NextProtos: []string{"http/1.1"},
		GetCertificate: func(hello *tls.ClientHelloInfo) (*tls.Certificate, error) {
			name := strings.ToLower(strings.TrimSpace(hello.ServerName))
			if c, ok := s.Leaves[name]; ok && c != nil {
				return c, nil
			}
			if s.DefaultLeaf != nil {
				return s.DefaultLeaf, nil
			}
			return nil, errors.New("mitm: leaf certificate unavailable")
		},
	}
	ln, err := tls.Listen("tcp", bindAddr, conf)
	if err != nil {
		return err
	}
	s.listener = ln
	return nil
}

func (s *Server) Addr() net.Addr {
	if s.listener == nil {
		return nil
	}
	return s.listener.Addr()
}

func (s *Server) Close() error {
	if s.listener == nil {
		return nil
	}
	err := s.listener.Close()
	s.listener = nil
	return err
}

func (s *Server) Serve(ctx context.Context) error {
	l := s.logger()
	shutdownDone := make(chan struct{})
	go func() {
		select {
		case <-ctx.Done():
			if s.listener != nil {
				_ = s.listener.Close()
			}
		case <-shutdownDone:
		}
	}()
	defer close(shutdownDone)

	for {
		cli, err := s.listener.Accept()
		if err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			if ne, ok := err.(net.Error); ok && ne.Timeout() {
				continue
			}
			return err
		}
		go s.handle(cli, l)
	}
}

func interceptSet(hosts []string) map[string]struct{} {
	m := make(map[string]struct{}, len(hosts))
	for _, h := range hosts {
		m[strings.ToLower(h)] = struct{}{}
	}
	return m
}

func (s *Server) handle(raw net.Conn, logf func(string, ...any)) {
	defer raw.Close()

	tlsCli, ok := raw.(*tls.Conn)
	if !ok {
		logf("mitm: expected *tls.Conn from listener layer")
		return
	}

	cliReader := bufio.NewReader(tlsCli)
	allowed := interceptSet(megiddo.InterceptHosts)

	type dialResult struct {
		conn   *tls.Conn
		reader *bufio.Reader
	}

	var session *dialResult
	defer func() {
		if session != nil && session.conn != nil {
			session.conn.Close()
		}
	}()

	for {
		req, err := http.ReadRequest(cliReader)
		if err != nil {
			if errors.Is(err, io.EOF) {
				return
			}
			return
		}

		host := canonicalHost(req.Host)
		addrKey := strings.ToLower(host)
		if _, ok := allowed[addrKey]; !ok {
			logf("mitm: unexpected host header %q", req.Host)
			_, _ = io.Copy(io.Discard, req.Body)
			req.Body.Close()
			return
		}

		if s.LocalFiles != nil && strings.HasPrefix(req.URL.Path, megiddo.LocalServePathPrefix) {
			_, _ = io.Copy(io.Discard, req.Body)
			req.Body.Close()
			if err := s.serveLocal(tlsCli, req, logf); err != nil {
				logf("mitm: local serve failed for %s: %v", req.URL.Path, err)
				return
			}
			continue
		}

		pool := s.UpstreamIPs[addrKey]
		if len(pool) == 0 {
			logf("mitm: missing upstream IPs for %s", host)
			_, _ = io.Copy(io.Discard, req.Body)
			req.Body.Close()
			return
		}

		if session == nil {
			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			upConn, err := s.dialUpstream(ctx, host, pool[0])
			cancel()
			if err != nil {
				logf("mitm: upstream dial failed: %v", err)
				_, _ = io.Copy(io.Discard, req.Body)
				req.Body.Close()
				return
			}
			session = &dialResult{conn: upConn, reader: bufio.NewReader(upConn)}
		}

		bufferBatch := assetbatch.BatchRequestPath(req.URL.Path) && s.Replacements != nil
		var fileMarks []assetbatch.FileMark
		if !bufferBatch {
			scrubRequestForProxy(host, req)
			if err := req.Write(session.conn); err != nil {
				_, _ = io.Copy(io.Discard, req.Body)
				req.Body.Close()
				return
			}
			_ = req.Body.Close()
		} else {
			payload, err := io.ReadAll(io.LimitReader(req.Body, assetbatch.WireReadLimit()))
			if err != nil {
				_, _ = io.Copy(io.Discard, req.Body)
				req.Body.Close()
				logf("mitm: reading client payload failed: %v", err)
				return
			}
			_, _ = io.Copy(io.Discard, req.Body)
			req.Body.Close()

			if len(payload) > assetbatch.MaxDecodedBatchBody {
				logf("megiddo: batch body larger than limit (%d bytes)", assetbatch.MaxDecodedBatchBody)
				if replyErr := replyRequestEntityTooLarge(tlsCli, req); replyErr != nil {
					logf("mitm: 413 reply failed: %v", replyErr)
				}
				return
			}

			res, rwErr := assetbatch.RewriteMITMBatch(payload, req.Header, s.Replacements, s.TexpackLookup, logf)
			if rwErr != nil && errors.Is(rwErr, assetbatch.ErrBatchBodyTooLarge) {
				logf("megiddo: oversized batch payload after decompress")
				if replyErr := replyRequestEntityTooLarge(tlsCli, req); replyErr != nil {
					logf("mitm: 413 reply failed: %v", replyErr)
				}
				return
			}

			toForward := payload
			hdrMutated := false
			if rwErr != nil {
				logf("megiddo: batch transformer gave up: %v", rwErr)
				if res.Body != nil {
					toForward = res.Body
				}
			} else {
				toForward = res.Body
				hdrMutated = res.Changed
				fileMarks = res.Files
			}

			if hdrMutated {
				assetbatch.PrepareBatchRequestHeader(req.Header, len(toForward))
			}
			req.Header.Del("Transfer-Encoding")
			req.ContentLength = int64(len(toForward))
			req.Header.Set("Content-Length", strconv.FormatInt(req.ContentLength, 10))
			scrubRequestForProxy(host, req)
			req.Body = io.NopCloser(bytes.NewReader(toForward))
			if err := req.Write(session.conn); err != nil {
				return
			}
		}
		resp, err := http.ReadResponse(session.reader, req)
		if err != nil {
			logf("mitm: read upstream response failed: %v", err)
			return
		}
		decorateCloseSemantics(req, resp)

		if len(fileMarks) > 0 && assetbatch.IsBatchResponseJSON(resp.Header.Get("Content-Type")) {
			if err := rewriteBatchResponse(resp, fileMarks, logf); err != nil {
				logf("megiddo: batch response rewrite failed: %v", err)
			}
		}

		if err := resp.Write(tlsCli); err != nil {
			_, _ = io.Copy(io.Discard, resp.Body)
			resp.Body.Close()
			return
		}
		_, _ = io.Copy(io.Discard, resp.Body)
		resp.Body.Close()

		if resp.Close {
			return
		}
	}
}

func rewriteBatchResponse(resp *http.Response, marks []assetbatch.FileMark, logf func(string, ...any)) error {
	raw, err := io.ReadAll(io.LimitReader(resp.Body, assetbatch.WireReadLimit()))
	if err != nil {
		return err
	}
	if int64(len(raw)) > int64(assetbatch.MaxDecodedBatchBody) {
		return assetbatch.ErrBatchBodyTooLarge
	}
	out, changed, err := assetbatch.RewriteMITMBatchResponse(raw, resp.Header, marks, logf)
	if err != nil {
		resp.Body = io.NopCloser(bytes.NewReader(raw))
		resp.ContentLength = int64(len(raw))
		resp.Header.Set("Content-Length", strconv.Itoa(len(raw)))
		return err
	}
	if !changed {
		resp.Body = io.NopCloser(bytes.NewReader(raw))
		resp.ContentLength = int64(len(raw))
		resp.Header.Set("Content-Length", strconv.Itoa(len(raw)))
		return nil
	}
	resp.Body = io.NopCloser(bytes.NewReader(out))
	resp.ContentLength = int64(len(out))
	resp.Header.Set("Content-Length", strconv.Itoa(len(out)))
	resp.Header.Del("Content-Encoding")
	resp.Header.Del("Transfer-Encoding")
	resp.Header.Set("Content-Type", "application/json")
	return nil
}

func (s *Server) serveLocal(w io.Writer, req *http.Request, logf func(string, ...any)) error {
	fp, ok := s.LocalFiles.Resolve(req.URL.Path)
	if !ok {
		return writeSimpleResponse(w, req, http.StatusNotFound, "application/octet-stream", nil)
	}
	data, err := os.ReadFile(fp)
	if err != nil {
		logf("megiddo: local file read %s: %v", fp, err)
		return writeSimpleResponse(w, req, http.StatusInternalServerError, "application/octet-stream", nil)
	}
	logf("megiddo: serving local %s (%d bytes) for %s", fp, len(data), req.URL.Path)
	return writeSimpleResponse(w, req, http.StatusOK, localserve.ContentTypeFor(fp), data)
}

func writeSimpleResponse(w io.Writer, req *http.Request, code int, ct string, body []byte) error {
	resp := &http.Response{
		StatusCode:    code,
		Status:        strconv.Itoa(code) + " " + http.StatusText(code),
		ProtoMajor:    req.ProtoMajor,
		ProtoMinor:    req.ProtoMinor,
		Proto:         req.Proto,
		Header:        make(http.Header),
		Body:          io.NopCloser(bytes.NewReader(body)),
		ContentLength: int64(len(body)),
	}
	resp.Header.Set("Content-Type", ct)
	resp.Header.Set("Content-Length", strconv.Itoa(len(body)))
	resp.Header.Set("Cache-Control", "no-store")
	return resp.Write(w)
}

func replyRequestEntityTooLarge(w io.Writer, req *http.Request) error {
	resp := &http.Response{
		StatusCode: http.StatusRequestEntityTooLarge,
		Status:     strconv.Itoa(http.StatusRequestEntityTooLarge) + " " + http.StatusText(http.StatusRequestEntityTooLarge),
		ProtoMajor: req.ProtoMajor,
		ProtoMinor: req.ProtoMinor,
		Proto:      req.Proto,
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader("")),
		Close:      true,
	}
	resp.ContentLength = 0
	return resp.Write(w)
}

func scrubRequestForProxy(host string, req *http.Request) {
	if req.URL == nil {
		req.URL = &url.URL{
			Scheme: "https",
			Host:   host,
			Path:   "/",
		}
	}
	req.RequestURI = ""
	if req.URL.Scheme == "" {
		req.URL.Scheme = "https"
	}
	if req.URL.Host == "" {
		req.URL.Host = host
	}
	req.Host = host
}

func decorateCloseSemantics(req *http.Request, resp *http.Response) {
	resp.Close = false
	cv := strings.ToLower(req.Header.Get("Connection"))
	if strings.Contains(cv, "close") ||
		strings.Contains(strings.ToLower(strings.TrimSpace(resp.Header.Get("Connection"))), "close") {
		resp.Close = true
		return
	}
	if strings.HasPrefix(strings.ToLower(req.Proto), "http/1.0") &&
		!strings.Contains(strings.ToLower(req.Header.Get("Connection")), "keep-alive") {
		resp.Close = true
	}
}
func canonicalHost(raw string) string {
	raw = strings.TrimSpace(strings.ToLower(raw))
	if strings.HasPrefix(raw, "[") {
		if idx := strings.Index(raw, "]"); idx > 1 {
			return raw[1:idx]
		}
	}
	host, _, err := net.SplitHostPort(raw)
	if err != nil {
		return strings.Trim(raw, "[]")
	}
	return strings.ToLower(host)
}

func (s *Server) dialUpstream(ctx context.Context, host, ip string) (*tls.Conn, error) {
	d := net.Dialer{}
	tcp, err := d.DialContext(ctx, "tcp", net.JoinHostPort(ip, "443"))
	if err != nil {
		return nil, err
	}
	cfg := &tls.Config{
		ServerName:         host,
		InsecureSkipVerify: true,
		MinVersion:         tls.VersionTLS12,
		NextProtos:         []string{"http/1.1"},
	}
	tConn := tls.Client(tcp, cfg)
	if err := tConn.HandshakeContext(ctx); err != nil {
		tcp.Close()
		return nil, err
	}
	return tConn, nil
}
