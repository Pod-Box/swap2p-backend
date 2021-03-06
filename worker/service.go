package worker

import (
	"context"
	"math/big"

	"github.com/Pod-Box/swap2p-backend/api"
	"github.com/pkg/errors"
	"github.com/umbracle/ethgo"
	"github.com/umbracle/ethgo/abi"
	"github.com/umbracle/ethgo/jsonrpc"
	"github.com/umbracle/ethgo/tracker"
)

type TradeEventType int

const (
	TradeEventTypeCreate TradeEventType = 1 + iota
	TradeEventTypeAccept
	TradeEventTypeReject
)

type TradeEvent struct {
	Type TradeEventType
	api.Trade
}

type Service struct {
	e20e20  *abi.ABI
	e20e20C ethgo.Address

	e20e721  *abi.ABI
	e20e721C ethgo.Address

	e721e20  *abi.ABI
	e721e20C ethgo.Address

	e721e721  *abi.ABI
	e721e721C ethgo.Address

	idName  map[string]*abi.Method
	idEvent map[string]*abi.Event

	TradeChan chan TradeEvent
	ErrChan   chan error

	jsonCli *jsonrpc.Client

	t *tracker.Tracker
}

func NewService(cfg *Config) (*Service, error) {
	s := &Service{
		TradeChan: make(chan TradeEvent),
		ErrChan:   make(chan error),
	}

	e20e20, err := abi.NewABI(cfg.E20E20)
	if err != nil {
		return nil, errors.Wrap(err, "new abi e20 -> e20")
	}
	e20e721, err := abi.NewABI(cfg.E20E721)
	if err != nil {
		return nil, errors.Wrap(err, "new abi e20 -> e721")
	}
	e721e20, err := abi.NewABI(cfg.E721E20)
	if err != nil {
		return nil, errors.Wrap(err, "new abi e721 -> e20")
	}
	e721e721, err := abi.NewABI(cfg.E721E721)
	if err != nil {
		return nil, errors.Wrap(err, "new abi e721 -> e721")
	}

	s.e20e20 = e20e20
	s.e20e721 = e20e721
	s.e721e20 = e721e20
	s.e721e721 = e721e721

	s.e20e20C = ethgo.HexToAddress(cfg.E20E20Contract)
	s.e20e721C = ethgo.HexToAddress(cfg.E20E721Contract)
	s.e721e20C = ethgo.HexToAddress(cfg.E721E20Contract)
	s.e721e721C = ethgo.HexToAddress(cfg.E721E721Contract)

	s.idName = map[string]*abi.Method{}
	s.idEvent = map[string]*abi.Event{}

	for _, v := range e20e20.Events {
		s.idEvent[v.ID().String()+s.e20e20C.String()] = v
	}
	for _, v := range e20e721.Events {
		s.idEvent[v.ID().String()+s.e20e721C.String()] = v
	}
	for _, v := range e721e20.Events {
		s.idEvent[v.ID().String()+s.e721e20C.String()] = v
	}
	for _, v := range e721e721.Events {
		s.idEvent[v.ID().String()+s.e721e721C.String()] = v
	}

	client, err := jsonrpc.NewClient(cfg.JSONRPCClient)
	if err != nil {
		return nil, errors.Wrap(err, "json rpc client")
	}

	s.jsonCli = client

	track, err := tracker.NewTracker(client.Eth(), tracker.WithFilter(&tracker.FilterConfig{
		Address: []ethgo.Address{s.e20e20C, s.e20e721C, s.e721e20C, s.e721e721C},
		Start:   cfg.BlockFrom,
	}))
	if err != nil {
		return nil, errors.Wrap(err, "tracker")
	}

	s.t = track

	return s, nil
}

func (s *Service) Run(ctx context.Context) {
	go func() {
		for evt := range s.t.EventCh {
			for _, e := range evt.Added {
				for _, t := range e.Topics {
					if v, ok := s.idEvent[t.String()+e.Address.String()]; ok {
						if e.Address != s.e20e20C && e.Address != s.e20e721C && e.Address != s.e721e20C && e.Address != s.e721e721C {
							continue
						}
						if e.Data == nil {
							continue
						}
						dataI, err := v.Inputs.Decode(e.Data)
						if err != nil {
							s.ErrChan <- err
							continue
						}
						data := dataI.(map[string]interface{})
						switch v.Name {
						case "EscrowCreated":
							id := data["escrowIndex"].(*big.Int)
							trade := api.Trade{
								Id:       int(id.Int64()),
								XAmount:  "0",
								YAmount:  "0",
								XAsset:   data["xTokenContractAddr"].(ethgo.Address).String(),
								YAsset:   data["yTokenContractAddr"].(ethgo.Address).String(),
								XAddress: data["xOwner"].(ethgo.Address).String(),
								YAddress: data["yOwner"].(ethgo.Address).String(),
							}
							switch e.Address {
							case s.e20e20C:
								trade.Type = api.TradeTypeN2020
								trade.XAmount = data["xAmount"].(*big.Int).String()
								trade.YAmount = data["yAmount"].(*big.Int).String()
								trade.YAsset = data["yTokenContractAddr"].(ethgo.Address).String()
								trade.XAsset = data["xTokenContractAddr"].(ethgo.Address).String()
							case s.e20e721C:
								trade.Type = api.TradeTypeN20721
								trade.XAmount = data["xAmount"].(*big.Int).String()
								trade.YAmount = data["yIndex"].(*big.Int).String()
							case s.e721e20C:
								trade.Type = api.TradeTypeN72120
								trade.XAmount = data["xIndex"].(*big.Int).String()
								trade.YAmount = data["yAmount"].(*big.Int).String()
							case s.e721e721C:
								trade.Type = api.TradeTypeN721721
								trade.XAmount = data["xIndex"].(*big.Int).String()
								trade.YAmount = data["yIndex"].(*big.Int).String()
							default:
								continue
							}
							s.TradeChan <- TradeEvent{
								Type:  TradeEventTypeCreate,
								Trade: trade,
							}
						case "EscrowAccepted":
							trade := api.Trade{
								Id:       int(data["escrowIndex"].(*big.Int).Int64()),
								YAddress: data["yOwner"].(ethgo.Address).String(),
							}
							switch e.Address {
							case s.e20e20C:
								trade.Type = api.TradeTypeN2020
							case s.e20e721C:
								trade.Type = api.TradeTypeN20721
							case s.e721e20C:
								trade.Type = api.TradeTypeN72120
							case s.e721e721C:
								trade.Type = api.TradeTypeN721721
							default:
								continue
							}
							s.TradeChan <- TradeEvent{
								Type:  TradeEventTypeAccept,
								Trade: trade,
							}
						case "EscrowRejected":
							s.TradeChan <- TradeEvent{
								Type: TradeEventTypeReject,
							}
						}
					}
				}
			}
		}
	}()
	s.t.Sync(ctx)
}
