package tracedredis

import (
	"context"
	"github.com/DataDog/dd-trace-go/tracer"
	redis "github.com/garyburd/redigo/redis"
	"net"
	"net/url"
	"strconv"
	"strings"
)

type TracedConn struct {
	redis.Conn
	tracer  *tracer.Tracer
	network string
	host    string
	port    string
}

func TracedDial(tracer *tracer.Tracer, network, address string, options ...redis.DialOption) (TracedConn, error) {
	c, err := redis.Dial(network, address)
	addr := strings.Split(address, ":")
	host := addr[0]
	port := addr[1]
	tc := TracedConn{c, tracer, network, host, port}
	return tc, err
}

func TracedDialURL(tracer *tracer.Tracer, rawurl string, options ...redis.DialOption) (TracedConn, error) {
	u, err := url.Parse(rawurl)
	if err != nil {
		return TracedConn{}, err
	}
	host, port, err := net.SplitHostPort(u.Host)
	if err != nil {
		host = u.Host
		port = "6379"
	}
	if host == "" {
		host = "localhost"
	}
	// Set in redis.DialUrl source code
	network := "tcp"
	c, err := redis.DialURL(rawurl, options...)
	tc := TracedConn{c, tracer, network, host, port}
	return tc, err
}

func (tc TracedConn) Do(commandName string, args ...interface{}) (reply interface{}, err error) {
	if len(args) == 0 {
		return tc.Conn.Do(commandName, args...)
	}
	ctx, ok := args[len(args)-1].(context.Context)
	if !ok {
		return tc.Conn.Do(commandName, args...)
	}
	args = args[:len(args)-1]
	span := tc.tracer.NewChildSpanFromContext("redis.command", ctx)
	span.SetMeta("out.network", tc.network)
	span.SetMeta("out.port", tc.port)
	span.SetMeta("out.host", tc.host)
	span.SetMeta("redis.args_length", strconv.Itoa(len(args)))

	raw_command := commandName
	for _, arg := range args {
		raw_command += " " + arg.(string)
	}
	span.SetMeta("redis.raw_command", raw_command)
	ret, err := tc.Conn.Do(commandName, args...)

	if err != nil {
		span.SetError(err)
	}
	span.Finish()

	return ret, err
}