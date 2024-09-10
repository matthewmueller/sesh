package sesh

import (
	"bytes"
	"encoding/gob"
)

type Codec interface {
	Encode(v any) ([]byte, error)
	Decode(data []byte, v any) error
}

type gobCodec struct {
}

func (g *gobCodec) Encode(v any) ([]byte, error) {
	b := new(bytes.Buffer)
	enc := gob.NewEncoder(b)
	if err := enc.Encode(v); err != nil {
		return nil, err
	}
	return b.Bytes(), nil
}

func (g *gobCodec) Decode(data []byte, v any) error {
	b := bytes.NewBuffer(data)
	dec := gob.NewDecoder(b)
	if err := dec.Decode(v); err != nil {
		return err
	}
	return nil
}
