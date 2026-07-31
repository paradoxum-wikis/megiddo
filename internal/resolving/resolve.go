package resolving

import (
	"context"
	"encoding/binary"
	"fmt"
	"net"
	"strings"
	"time"
)

func RoutableIPv4(ip net.IP) bool {
	v4 := ip.To4()
	if v4 == nil {
		return false
	}
	if !v4.IsGlobalUnicast() {
		return false
	}
	// skip cgnat 100.64.0.0/10 (vpn overlays often resolve here)
	if v4[0] == 100 && v4[1] >= 64 && v4[1] <= 127 {
		return false
	}
	return true
}

func RealIPs(ctx context.Context, hostname string) ([]string, error) {
	r := net.Resolver{}
	addrs, err := r.LookupIPAddr(ctx, hostname)
	var out []string
	if err == nil {
		for _, a := range addrs {
			if RoutableIPv4(a.IP) {
				out = append(out, a.IP.String())
			}
		}
		out = dedupePreserve(out)
	}
	if len(out) > 0 {
		return out, nil
	}

	lastErr := err
	for _, srv := range []string{"8.8.8.8:53", "1.1.1.1:53", "1.0.0.1:53"} {
		ctx2, cancel := context.WithTimeout(ctx, 4*time.Second)
		ips, err := dnsUDPQueryA(ctx2, srv, hostname)
		cancel()
		if err != nil {
			lastErr = err
			continue
		}
		if len(ips) > 0 {
			return ips, nil
		}
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("no usable IPv4 addresses")
	}
	return nil, fmt.Errorf("%s: %w", hostname, lastErr)
}

func dedupePreserve(in []string) []string {
	if len(in) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(in))
	o := make([]string, 0, len(in))
	for _, ip := range in {
		if _, ok := seen[ip]; ok {
			continue
		}
		seen[ip] = struct{}{}
		o = append(o, ip)
	}
	return o
}

func dnsUDPQueryA(ctx context.Context, serverAddr, hostname string) ([]string, error) {
	d := net.Dialer{}
	pc, err := d.DialContext(ctx, "udp", serverAddr)
	if err != nil {
		return nil, err
	}
	defer pc.Close()
	if dl, ok := ctx.Deadline(); ok {
		_ = pc.SetDeadline(dl)
	}
	q, err := buildDNSQuery(hostname)
	if err != nil {
		return nil, err
	}
	if _, err := pc.Write(q); err != nil {
		return nil, err
	}
	buf := make([]byte, 4096)
	n, err := pc.Read(buf)
	if err != nil {
		return nil, err
	}
	ips := parseAnswersA(buf[:n])
	filtered := filteredRoutable(ips)
	return dedupePreserve(filtered), nil
}

func buildDNSQuery(hostname string) ([]byte, error) {
	hostname = strings.TrimSuffix(hostname, ".")
	if hostname == "" {
		return nil, fmt.Errorf("empty hostname")
	}
	const txid = uint16(0x4d47)
	hdr := []byte{
		byte((txid >> 8) & 0xff),
		byte(txid & 0xff),
		0x01, 0x00, // flags such as standard query + recursion desired
		0x00, 0x01, // one question
		0x00, 0x00, // answer RRs
		0x00, 0x00,
		0x00, 0x00,
	}
	var qb []byte
	for _, lbl := range strings.Split(hostname, ".") {
		if len(lbl) == 0 {
			continue
		}
		if len(lbl) > 63 {
			return nil, fmt.Errorf("label too long in %s", hostname)
		}
		qb = append(qb, byte(len(lbl)))
		qb = append(qb, lbl...)
	}
	qb = append(qb, 0)
	qb = append(qb, 0x00, 0x01, 0x00, 0x01) // A, IN

	return append(hdr, qb...), nil
}

func parseAnswersA(packet []byte) []string {
	if len(packet) < 12 {
		return nil
	}
	qdcount := binary.BigEndian.Uint16(packet[4:])
	ancount := binary.BigEndian.Uint16(packet[6:])
	pos := 12

	skipQName := func() int {
		for pos < len(packet) {
			l := packet[pos]
			if l == 0 {
				pos++
				return pos
			}
			if l&0xC0 == 0xC0 {
				pos += 2
				return pos
			}
			pos++
			pos += int(l)
		}
		return pos
	}

	for ; qdcount > 0; qdcount-- {
		pos = skipQName()
		if pos+4 > len(packet) {
			return nil
		}
		pos += 4 // type + class
	}

	var ips []string
	for ; ancount > 0; ancount-- {
		pos = skipQName()
		if pos+10 > len(packet) {
			break
		}
		rdtype := binary.BigEndian.Uint16(packet[pos:])
		rdlen := binary.BigEndian.Uint16(packet[pos+8:])
		pos += 10
		if int(rdlen) < 0 || pos+int(rdlen) > len(packet) {
			break
		}
		if rdtype == 1 && rdlen == 4 {
			ip := fmt.Sprintf("%d.%d.%d.%d",
				packet[pos], packet[pos+1], packet[pos+2], packet[pos+3])
			ips = append(ips, ip)
		}
		pos += int(rdlen)
	}
	return ips
}

func filteredRoutable(in []string) []string {
	out := in[:0]
	for _, ip := range in {
		if RoutableIPv4(net.ParseIP(ip)) {
			out = append(out, ip)
		}
	}
	return out
}
