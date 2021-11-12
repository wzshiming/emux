package emux

import (
	"encoding/binary"
	"fmt"
	"io"
)

var (
	ErrInvalidStream = fmt.Errorf("invalid stream")
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

func (e *Encode) WriteCmd(cmd uint8, sid uint64) error {
	var buf [binary.MaxVarintLen64 + 1]byte
	buf[0] = cmd
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
		return 0, fmt.Errorf("read uvarint: %w: %s", ErrInvalidStream, err)
	}

	n, err := io.CopyBuffer(w, io.LimitReader(d.r, int64(i)), buf)
	if err != nil {
		if l := int64(i) - n; l > 0 && w != io.Discard {
			k, err0 := io.CopyBuffer(io.Discard, io.LimitReader(d.r, l), buf)
			if err0 != nil {
				if k != l {
					err = fmt.Errorf("%s: %w: %s", err, ErrInvalidStream, err0)
				} else {
					err = fmt.Errorf("%w: %s", err, err0)
				}
			} else {
				if k != l {
					err = fmt.Errorf("%s: %w", err, ErrInvalidStream)
				}
			}
		}
		return int64(i), err
	}
	if l := int64(i) - n; l > 0 {
		if w != io.Discard {
			k, err := io.CopyBuffer(io.Discard, io.LimitReader(d.r, l), buf)
			if err != nil {
				if k != l {
					err = fmt.Errorf("%w: %s", ErrInvalidStream, err)
				}
				return int64(i), err
			}
		} else {
			return int64(i), fmt.Errorf("emux: %w", ErrInvalidStream)
		}
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
