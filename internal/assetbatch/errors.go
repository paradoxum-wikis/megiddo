package assetbatch

import "errors"

var ErrBatchBodyTooLarge = errors.New("assetbatch: batch body exceeds configured limit")
