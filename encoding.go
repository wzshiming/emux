package emux

import (
	"encoding/binary"
	"io"
)

type Encode struct {
	w io.Writer
}

func NewEncode(w io.Writer) *Encode {
	return &Encode{
		w: w,
	}
}

func (e *Encode) WriteUvarint(v uint64) error {
	var buf [binary.MaxVarintLen64]byte
	n := binary.PutUvarint(buf[:], v)
	_, err := e.w.Write(buf[:n])
	return err
}

func (e *Encode) WriteBytes(b []byte) error {
	err := e.WriteUvarint(uint64(len(b)))
	if err != nil {
		return err
	}
	if len(b) > 0 {
		_, err = e.w.Write(b)
	}
	return err
}

func (e *Encode) WriteByte(b byte) error {
	var buf = [...]byte{b}
	_, err := e.w.Write(buf[:])
	return err
}

func (e *Encode) WriteCmd(cmd Cmd, sid uint64) error {
	var buf [binary.MaxVarintLen64 + 1]byte
	buf[0] = byte(cmd)
	n := binary.PutUvarint(buf[1:], sid)
	_, err := e.w.Write(buf[:n+1])
	return err
}

func (e *Encode) Flush() error {
	if f, ok := e.w.(interface{ Flush() error }); ok {
		return f.Flush()
	}
	return nil
}

type Decode struct {
	r io.Reader
}

func NewDecode(r io.Reader) *Decode {
	return &Decode{
		r: r,
	}
}

func (d *Decode) ReadUvarint() (uint64, error) {
	return binary.ReadUvarint(d)
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
	_, err = io.ReadFull(d.r, buf)
	return buf, err
}

func (d *Decode) WriteTo(w io.Writer, buf []byte) (int64, error) {
	i, err := d.ReadUvarint()
	if err != nil {
		return 0, err
	}

	_, err = io.CopyBuffer(w, io.LimitReader(d.r, int64(i)), buf)
	if err != nil {
		return 0, err
	}
	return int64(i), nil
}

func (d *Decode) ReadByte() (byte, error) {
	if rb, ok := d.r.(io.ByteReader); ok {
		return rb.ReadByte()
	}
	var buf [1]byte
	_, err := d.r.Read(buf[:])
	return buf[0], err
}
