package driver

import "context"

type MessageHandler func(topic string, payload []byte)

type Driver interface {
	Connect(ctx context.Context) (bool, error)
	Disconnect(ctx context.Context) error
	IsConnected() bool
	ExecuteAction(ctx context.Context, action string, params map[string]any) (any, error)
	RegisterMessageHandler(handler MessageHandler)
	LastError() string
}
