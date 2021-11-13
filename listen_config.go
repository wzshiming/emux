package emux

import (
	"context"
	"fmt"
	"net"
)

type ListenConfigSession struct {
	listenConfig ListenConfig
	Logger       Logger
	BytesPool    BytesPool
	Handshake    Handshake
	Instruction  Instruction
}

func NewListenConfig(listener ListenConfig) *ListenConfigSession {
	return &ListenConfigSession{
		listenConfig: listener,
		Handshake:    DefaultServerHandshake,
		Instruction:  DefaultInstruction,
	}
}

func (l *ListenConfigSession) Listen(ctx context.Context, network, address string) (net.Listener, error) {
	if l.listenConfig == nil {
		return nil, fmt.Errorf("does not support the listen")
	}
	listener, err := l.listenConfig.Listen(ctx, network, address)
	if err != nil {
		return nil, err
	}
	lt := NewListener(ctx, listener)
	lt.Logger = l.Logger
	lt.BytesPool = l.BytesPool
	lt.Handshake = l.Handshake
	lt.Instruction = l.Instruction
	return lt, nil
}
