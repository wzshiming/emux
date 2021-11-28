package emux

import (
	"encoding/binary"
	"io"
)

type Encode struct {
	io.Writer
	buf [binary.MaxVarintLen64 + 1]byte
}

func NewEncode(w io.Writer) *Encode {
	return &Encode{
		Writer: w,
	}
}

func (e *Encode) WriteUvarint(v uint64) error {
	n := binary.PutUvarint(e.buf[:], v)
	_, err := e.Write(e.buf[:n])
	return err
}

func (e *Encode) WriteBytes(b []byte) error {
	err := e.WriteUvarint(uint64(len(b)))
	if err != nil {
		return err
	}
	if len(b) > 0 {
		_, err = e.Write(b)
	}
	return err
}

func (e *Encode) WriteByte(b byte) error {
	e.buf[0] = b
	_, err := e.Write(e.buf[:1])
	return err
}

func (e *Encode) WriteCmd(cmd uint8, sid uint64) error {
	e.buf[0] = cmd
	n := binary.PutUvarint(e.buf[1:], sid)
	_, err := e.Write(e.buf[:n+1])
	return err
}

type Decode struct {
	io.Reader
	buf [1]byte
}

func NewDecode(r io.Reader) *Decode {
	return &Decode{
		Reader: r,
	}
}

func (d *Decode) ReadUvarint() (uint64, error) {
	return binary.ReadUvarint(d)
}

func (d *Decode) ReadByte() (byte, error) {
	_, err := d.Read(d.buf[:1])
	if err != nil {
		return 0, err
	}
	return d.buf[0], nil
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
	_, err = io.ReadFull(d, buf)
	return buf, err
}

func (d *Decode) WriteTo(w io.Writer, buf []byte) (int64, error) {
	i, err := d.ReadUvarint()
	if err != nil {
		return 0, err
	}
	return io.CopyBuffer(w, io.LimitReader(d, int64(i)), buf)
}
