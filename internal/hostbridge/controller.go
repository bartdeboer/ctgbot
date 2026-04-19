package hostbridge

import (
	"context"
	"crypto/tls"
	"log"
	"net"

	"github.com/bartdeboer/ctgbot/internal/hostbridge/server"
)

type AllowedCommand = server.AllowedCommand
type AllowedCommandResolver = server.AllowedCommandResolver

type SendFileRequest = server.SendFileRequest
type SendFileHandler = server.SendFileHandler

type SendTextRequest = server.SendTextRequest
type SendTextHandler = server.SendTextHandler

type ConfigListRequest = server.ConfigListRequest
type ConfigListHandler = server.ConfigListHandler

type ConfigSetRequest = server.ConfigSetRequest
type ConfigSetHandler = server.ConfigSetHandler

type executionPlan = server.ExecutionPlan

type tlsListenerConfig interface{ HostbridgeTCPListenAddr() string }

func Serve(ctx context.Context, address string, defaultTimeoutSec int, allowed map[string]AllowedCommand, sendFile SendFileHandler, sendText SendTextHandler, configList ConfigListHandler, configSet ConfigSetHandler, logger *log.Logger) error {
	return server.Serve(ctx, address, defaultTimeoutSec, allowed, sendFile, sendText, configList, configSet, logger)
}
func ServeListener(ctx context.Context, ln net.Listener, defaultTimeoutSec int, resolve AllowedCommandResolver, sendFile SendFileHandler, sendText SendTextHandler, configList ConfigListHandler, configSet ConfigSetHandler, logger *log.Logger) error {
	return server.ServeListener(ctx, ln, defaultTimeoutSec, resolve, sendFile, sendText, configList, configSet, logger)
}
func Listen(address string) (net.Listener, error) { return server.Listen(address) }
func ListenTLS(address string, tlsConfig *tls.Config) (net.Listener, error) {
	return server.ListenTLS(address, tlsConfig)
}
func NewTLSListener(cfg tlsListenerConfig, tlsConfig *tls.Config) (net.Listener, error) {
	return server.NewTLSListener(cfg, tlsConfig)
}
func buildExecutionPlan(req Request, spec AllowedCommand) (executionPlan, error) {
	return server.BuildExecutionPlan(req, spec)
}
func handleConn(conn net.Conn, resolve AllowedCommandResolver, sendFile SendFileHandler, sendText SendTextHandler, configList ConfigListHandler, configSet ConfigSetHandler, defaultTimeoutSec int, logger *log.Logger) {
	server.HandleConn(conn, resolve, sendFile, sendText, configList, configSet, defaultTimeoutSec, logger)
}
func DefaultAllowedCommands() map[string]AllowedCommand { return server.DefaultAllowedCommands() }
func MergeAllowedCommands(extra map[string]string) map[string]AllowedCommand {
	return server.MergeAllowedCommands(extra)
}
func MergeNamedAllowedCommands(extra map[string]AllowedCommand) map[string]AllowedCommand {
	return server.MergeNamedAllowedCommands(extra)
}
func MergeAllowedCommandSpecs(specs []string) map[string]AllowedCommand {
	return server.MergeAllowedCommandSpecs(specs)
}
func AllowedCommandsFromSpecs(specs []string) map[string]AllowedCommand {
	return server.AllowedCommandsFromSpecs(specs)
}
func AllowedCommandNames(allowed map[string]AllowedCommand) []string {
	return server.AllowedCommandNames(allowed)
}
func StaticAllowedCommandResolver(allowed map[string]AllowedCommand) AllowedCommandResolver {
	return server.StaticAllowedCommandResolver(allowed)
}
