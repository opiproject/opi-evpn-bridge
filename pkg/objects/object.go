package objects

import (
	"context"
	"github.com/opiproject/opi-evpn-bridge/pkg/utils"
	"github.com/philippgille/gokv"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/trace"
	"log"
)

type ObjectFactory[T any] interface {
	Create(ctx context.Context, in interface{}) (T, error)
	Delete(ctx context.Context, in interface{}) error
	Update(ctx context.Context, in interface{}) (T, error)
	Get(ctx context.Context, in interface{}) (T, error)
	List(ctx context.Context, in interface{}) ([]T, error)
}

const (
	TenantbridgeName = "br-tenant"
)

type Server struct {
	Pagination map[string]int
	ListHelper map[string]bool
	NLink      utils.Netlink
	Frr        utils.Frr
	Tracer     trace.Tracer
	Store      gokv.Store
}

// NewServer creates initialized instance of EVPN server
func NewServer(store gokv.Store) *Server {
	nLink := utils.NewNetlinkWrapper()
	frr := utils.NewFrrWrapper()
	return NewServerWithArgs(nLink, frr, store)
}

// NewServerWithArgs creates initialized instance of EVPN server
// with externally created Netlink
func NewServerWithArgs(nLink utils.Netlink, frr utils.Frr, store gokv.Store) *Server {
	if frr == nil {
		log.Panic("nil for Frr is not allowed")
	}
	if nLink == nil {
		log.Panic("nil for Netlink is not allowed")
	}
	if store == nil {
		log.Panic("nil for Store is not allowed")
	}
	return &Server{
		ListHelper: make(map[string]bool),
		Pagination: make(map[string]int),
		nLink:      nLink,
		frr:        frr,
		tracer:     otel.Tracer(""),
		store:      store,
	}
}
