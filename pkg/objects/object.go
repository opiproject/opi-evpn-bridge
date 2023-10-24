package objects

import (
	"fmt"
	"github.com/google/uuid"
	"github.com/opiproject/opi-evpn-bridge/pkg/models"
	"github.com/opiproject/opi-evpn-bridge/pkg/utils"
	"github.com/philippgille/gokv"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/trace"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"log"
	"sort"
	"strings"
)

type ObjectOps[T any] interface {
	Delete(o *models.EvpnObject[T]) error
	Create(new *models.EvpnObject[T]) error
	Set(old *models.EvpnObject[T], new *models.EvpnObject[T]) error
	Get(k string) (v *models.EvpnObject[T], err error)
	List(pSize int32, pToken string, prefix string) ([]T, string, error)
}

const (
	TenantbridgeName = "br-tenant"
)

type Server[T any] struct {
	Pagination map[string]int
	ListHelper map[string]bool
	NLink      utils.Netlink
	Frr        utils.Frr
	Tracer     trace.Tracer
	Store      gokv.Store
}

// NewServer creates initialized instance of EVPN server
func NewServer[T any](store gokv.Store) *Server[T] {
	nLink := utils.NewNetlinkWrapper()
	frr := utils.NewFrrWrapper()
	return NewServerWithArgs[T](nLink, frr, store)
}

// NewServerWithArgs creates initialized instance of EVPN server
// with externally created Netlink
func NewServerWithArgs[T any](nLink utils.Netlink, frr utils.Frr, store gokv.Store) *Server[T] {
	if frr == nil {
		log.Panic("nil for Frr is not allowed")
	}
	if nLink == nil {
		log.Panic("nil for Netlink is not allowed")
	}
	if store == nil {
		log.Panic("nil for Store is not allowed")
	}
	return &Server[T]{
		ListHelper: make(map[string]bool),
		Pagination: make(map[string]int),
		NLink:      nLink,
		Frr:        frr,
		Tracer:     otel.Tracer(""),
		Store:      store,
	}
}

func (s *Server[T]) Delete(o *models.EvpnObject[T]) error {
	name := (*o).GetName()
	delete(s.ListHelper, name)
	return s.Store.Delete(name)
}

func (s *Server[T]) Create(new *models.EvpnObject[T]) error {
	s.ListHelper[(*new).GetName()] = false
	return s.Store.Set((*new).GetName(), *new)
}

func (s *Server[T]) Set(old *models.EvpnObject[T], new *models.EvpnObject[T]) error {
	return s.Store.Set((*old).GetName(), *new)
}

func (s *Server[T]) Get(k string) (v *models.EvpnObject[T], ok bool, err error) {
	ok, err = s.Store.Get(k, v)
	if err != nil {
		fmt.Printf("Failed to interact with store: %v", err)
		return nil, false, err
	}
	if !ok {
		err := status.Errorf(codes.NotFound, "unable to find key %s", k)
		return nil, ok, err
	}
	return v, ok, nil
}

func (s *Server[T]) List(pSize int32, pToken string, prefix string) ([]T, string, error) {
	// fetch pagination from the database, calculate size and offset
	size, offset, perr := utils.ExtractPagination(pSize, pToken, s.Pagination)
	if perr != nil {
		return nil, "", perr
	}
	// fetch object from the database
	var Blobarray []*models.EvpnObject[T]
	for key := range s.ListHelper {
		if !strings.HasPrefix(key, prefix) {
			continue
		}
		o := new(models.EvpnObject[T])
		ok, err := s.Store.Get(key, o)
		if err != nil {
			fmt.Printf("Failed to interact with store: %v", err)
			return nil, "", err
		}
		if !ok {
			err := status.Errorf(codes.NotFound, "unable to find key %s", key)
			return nil, "", err
		}
		Blobarray = append(Blobarray, o)
	}
	// sort is needed, since MAP is unsorted in golang, and we might get different results
	sort.Slice(Blobarray, func(i, j int) bool {
		return (*Blobarray[i]).GetName() < (*Blobarray[j]).GetName()
	})
	log.Printf("Limiting result len(%d) to [%d:%d]", len(Blobarray), offset, size)
	Blobarray, hasMoreElements := utils.LimitPagination(Blobarray, offset, size)
	token := ""
	if hasMoreElements {
		token = uuid.New().String()
		s.Pagination[token] = offset + size
	}

	return s.convertToPbSlice(Blobarray), token, nil
}

func (s *Server[T]) convertToPbSlice(a []*models.EvpnObject[T]) []T {
	res := make([]T, 0)
	for _, i := range a {
		pb := (*i).ToPb()
		res = append(res, pb)
	}
	return res
}
