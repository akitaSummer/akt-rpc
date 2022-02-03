package xclient

import (
	Client "akt-rpc/client"
	"akt-rpc/option"
	"context"
	"io"
	"reflect"
	"sync"
)

type XClient struct {
	d       Discovery
	mode    SelectMode
	opt     *option.Option
	mu      sync.Mutex
	clients map[string]*Client.Client
}

var _ io.Closer = (*XClient)(nil)

func NewXClient(d Discovery, mode SelectMode, opt *option.Option) *XClient {
	return &XClient{d: d, mode: mode, opt: opt, clients: make(map[string]*Client.Client)}
}

func (xc *XClient) Close() error {
	xc.mu.Lock()
	defer xc.mu.Unlock()
	for key, client := range xc.clients {
		_ = client.Close()
		delete(xc.clients, key)
	}
	return nil
}

func (xc *XClient) dial(rpcAddr string) (*Client.Client, error) {
	xc.mu.Lock()
	defer xc.mu.Unlock()
	client, ok := xc.clients[rpcAddr]
	if ok && !client.IsAvailable() {
		_ = client.Close()
		delete(xc.clients, rpcAddr)
		client = nil
	}
	if client == nil {
		var err error
		client, err = Client.XDial(rpcAddr, xc.opt)
		if err != nil {
			return nil, err
		}
		xc.clients[rpcAddr] = client
	}
	return client, nil
}

func (xc *XClient) call(rpcAddr string, ctx context.Context, serviceMethod string, args, reply interface{}) error {
	client, err := xc.dial(rpcAddr)
	if err != nil {
		return err
	}
	return client.Call(ctx, serviceMethod, args, reply)
}

func (xc *XClient) Call(ctx context.Context, serviceMethod string, args, reply interface{}) error {
	rpcAddr, err := xc.d.Get(xc.mode)
	if err != nil {
		return err
	}
	return xc.call(rpcAddr, ctx, serviceMethod, args, reply)
}

// 将请求广播到所有的服务实例
func (xc *XClient) Broadcast(ctx context.Context, serviceMethod string, args, reply interface{}) error {
	servers, err := xc.d.GetAll()
	if err != nil {
		return err
	}
	var (
		wg        sync.WaitGroup
		mu        sync.Mutex
		e         error
		replyDone bool = reply == nil // 如果 reply 为 nil 则不需要设值
	)
	ctx, cancel := context.WithCancel(ctx)
	for _, rpcAddr := range servers {
		wg.Add(1)
		go func(rpcAddr string) {
			defer wg.Done()
			var clonedReply interface{}
			if reply != nil {
				clonedReply = reflect.New(reflect.ValueOf(reply).Elem().Type()).Interface()
			}
			err := xc.call(rpcAddr, ctx, serviceMethod, args, clonedReply)
			mu.Lock()
			if err != nil && e == nil {
				e = err
				cancel() // 如果有一个失败则取消所有请求，并返回失败数据
			}
			if err == nil && !replyDone {
				reflect.ValueOf(reply).Elem().Set(reflect.ValueOf(clonedReply).Elem())
				replyDone = true
			}
			mu.Unlock()
		}(rpcAddr)
	}
	wg.Wait()
	return e

}
