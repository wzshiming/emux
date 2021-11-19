package emux

import (
	"context"
	"fmt"
	"net"
	"time"
)

type ListenConfigSession struct {
	ctx          context.Context
	listenConfig ListenConfig
	Logger       Logger
	BytesPool    BytesPool
	Handshake    Handshake
	Instruction  Instruction
	Timeout      time.Duration
}

func NewListenConfig(ctx context.Context, listener ListenConfig) *ListenConfigSession {
	return &ListenConfigSession{
		ctx:          ctx,
		listenConfig: listener,
		Handshake:    DefaultServerHandshake,
		Instruction:  DefaultInstruction,
		Timeout:      DefaultTimeout,
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
	lt := NewListener(l.ctx, listener)
	lt.Logger = l.Logger
	lt.BytesPool = l.BytesPool
	lt.Handshake = l.Handshake
	lt.Instruction = l.Instruction
	lt.Timeout = l.Timeout
	return lt, nil
}
