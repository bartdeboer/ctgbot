package hostbridge

import hbprotocol "github.com/bartdeboer/ctgbot/internal/hostbridge/protocol"

type Operation = hbprotocol.Operation

const (
	OpRunCommand   = hbprotocol.OpRunCommand
	OpSendFile     = hbprotocol.OpSendFile
	OpSendText     = hbprotocol.OpSendText
	MaxSendFileBytes = hbprotocol.MaxSendFileBytes
)

type Request = hbprotocol.Request
type StreamKind = hbprotocol.StreamKind
type Frame = hbprotocol.Frame

const (
	StreamStdout = hbprotocol.StreamStdout
	StreamStderr = hbprotocol.StreamStderr
	StreamExit   = hbprotocol.StreamExit
	StreamError  = hbprotocol.StreamError
)
