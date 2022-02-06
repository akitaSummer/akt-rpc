package registry

import (
	"log"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"
)

// 添加服务器并接收心跳以使其保持活动状态。
// 返回所有活着的服务器并同步删除死服务器。
type AktRegistry struct {
	timeout time.Duration
	mu      sync.Mutex
	servers map[string]*ServerItem
}

type ServerItem struct {
	Addr  string
	start time.Time
}

const (
	defaultPath    = "/_aktrpc_/registry"
	defaultTimeout = time.Minute * 5
)

func New(timeout time.Duration) *AktRegistry {
	return &AktRegistry{
		servers: make(map[string]*ServerItem),
		timeout: timeout,
	}
}

var DefaultRegister = New(defaultTimeout)

// 添加服务实例，如果服务已经存在，则更新 start
func (r *AktRegistry) putServer(addr string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	s := r.servers[addr]
	if s == nil {
		r.servers[addr] = &ServerItem{Addr: addr, start: time.Now()}
	} else {
		s.start = time.Now()
	}
}

// 返回可用的服务列表，如果存在超时的服务，则删除。
func (r *AktRegistry) aliveServers() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	var alive []string
	for addr, s := range r.servers {
		if r.timeout == 0 || s.start.Add(r.timeout).After(time.Now()) {
			alive = append(alive, addr)
		} else {
			delete(r.servers, addr)
		}
	}
	sort.Strings(alive)
	return alive
}

func (r *AktRegistry) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	switch req.Method {
	// 获取所有实例
	case "GET":
		w.Header().Set("X-Aktrpc-Servers", strings.Join(r.aliveServers(), ","))
	// 添加实例
	case "POST":
		addr := req.Header.Get("X-Aktrpc-Server")
		if addr == "" {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		r.putServer(addr)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (r *AktRegistry) HandleHTTP(registryPath string) {
	http.Handle(registryPath, r)
	log.Println("rpc registry path:", registryPath)
}

func HandleHTTP() {
	DefaultRegister.HandleHTTP(defaultPath)
}

// 每隔一段时间发送一个心跳消息
func Heartbeat(registry, addr string, duration time.Duration) {
	if duration == 0 {
		duration = defaultTimeout - time.Duration(1)*time.Minute
	}
	var err error
	err = sendHeartbeat(registry, addr)
	go func() {
		t := time.NewTicker(duration)
		for err == nil {
			<-t.C
			err = sendHeartbeat(registry, addr)
		}
	}()
}

func sendHeartbeat(registry, addr string) error {
	log.Println(addr, "send heart beat to registry", registry)
	httpClient := &http.Client{}
	req, _ := http.NewRequest("POST", registry, nil)
	req.Header.Set("X-Aktrpc-Server", addr)
	if _, err := httpClient.Do(req); err != nil {
		log.Println("rpc server: heart beat err:", err)
		return err
	}
	return nil
}
