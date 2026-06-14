package p2p

import (
	"encoding/binary"
	"fmt"
	"io"
	"log"
)

const MaxPayloadSize int64 = 256 * 1024 * 1024

type Decoder interface {
    Decode(io.Reader, *RPC) error
}

type DefaultDecoder struct {
}

func (dec *DefaultDecoder) Decode(r io.Reader, rpc *RPC) error{
    var incomingDataType byte
    incomingDataTypeBytes := make([]byte, 1)
    n, err := r.Read(incomingDataTypeBytes)
    if err != nil {
        log.Println(err)
        return err
    }
    if n == 0 {
        return fmt.Errorf("read 0 bytes for incoming data type")
    }
    incomingDataType = incomingDataTypeBytes[0]
    if incomingDataType==IncomingStream {
        rpc.Stream = true
        return nil 
    }

    var contentLength int64
    if err := binary.Read(r, binary.LittleEndian, &contentLength); err != nil {
        log.Printf("failed to read content length: %v", err)
        return err
    }

    if contentLength <= 0 || contentLength > MaxPayloadSize {
        return fmt.Errorf("invalid payload size %d (max %d)", contentLength, MaxPayloadSize)
    }
    rpc.Size = contentLength

    buf := make([]byte, contentLength)
    n, err = r.Read(buf)
    if err != nil {
        return err
    }
    rpc.Payload = buf[:n]

    return nil
}

