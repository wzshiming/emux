package emux

import (
	"encoding/binary"
	"io"
)

type DecodeReader interface {
	io.Reader
	io.ByteReader
}

type EncodeWriter interface {
	io.Writer
	Flush() error
}

type Encode struct {
	buf [binary.MaxVarintLen64 + 1]byte
	EncodeWriter
}

func NewEncode(w EncodeWriter) *Encode {
	return &Encode{
		EncodeWriter: w,
	}
}

func (e *Encode) WriteUvarint(v uint64) error {
	n := binary.PutUvarint(e.buf[:], v)
	_, err := e.EncodeWriter.Write(e.buf[:n])
	return err
}

func (e *Encode) WriteBytes(b []byte) error {
	err := e.WriteUvarint(uint64(len(b)))
	if err != nil {
		return err
	}
	if len(b) > 0 {
		_, err = e.EncodeWriter.Write(b)
	}
	return err
}

func (e *Encode) WriteByte(b byte) error {
	e.buf[0] = b
	_, err := e.EncodeWriter.Write(e.buf[:1])
	return err
}

func (e *Encode) WriteCmd(cmd uint8, sid uint64) error {
	e.buf[0] = cmd
	n := binary.PutUvarint(e.buf[1:], sid)
	_, err := e.EncodeWriter.Write(e.buf[:n+1])
	return err
}

type Decode struct {
	DecodeReader
}

func NewDecode(r DecodeReader) *Decode {
	return &Decode{
		DecodeReader: r,
	}
}

func (d *Decode) ReadUvarint() (uint64, error) {
	return binary.ReadUvarint(d.DecodeReader)
}

func (d *Decode) ReadBytes() ([]byte, error) {
	i, err := d.ReadUvarint()
	if err != nil {
		return nil, err
	}
	if i == 0 {
		return []byte{}, nil
	}
	buf := make([]byte, i)
	_, err = io.ReadFull(d.DecodeReader, buf)
	return buf, err
}

func (d *Decode) WriteTo(w io.Writer, buf []byte) (int64, error) {
	i, err := d.ReadUvarint()
	if err != nil {
		return 0, err
	}
	return io.CopyBuffer(w, io.LimitReader(d.DecodeReader, int64(i)), buf)
}
