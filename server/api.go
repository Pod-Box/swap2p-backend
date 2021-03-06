package server

import (
	"database/sql"
	"net/http"

	"github.com/Pod-Box/swap2p-backend/api"
	"github.com/Pod-Box/swap2p-backend/repo"
	"github.com/go-chi/render"
	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
	"github.com/umbracle/ethgo"
)

func (s *Server) AddAsset(w http.ResponseWriter, r *http.Request, params api.AddAssetParams) {
	ctx := r.Context()
	name, dec, err := s.ass.GetAssetData(string(params.Asset))
	if err != nil {
		respond(w, r, s.log, errors.Wrap(err, "get asset data"))
		return
	}
	err = s.db.AddAsset(ctx, string(params.Asset), name, dec)
	if err != nil {
		respond(w, r, s.log, errors.Wrap(err, "add asset"))
		return
	}
}

func (s *Server) GetAssetsByAddress(w http.ResponseWriter, r *http.Request, params api.GetAssetsByAddressParams) {
	ctx := r.Context()
	al, err := s.db.GetBalancesByAddress(ctx, string(params.Wallet))
	if err != nil {
		respond(w, r, s.log, errors.Wrap(err, "assets by address"))
		return
	}
	respond(w, r, s.log, al)
}

func (s *Server) GetAllAssets(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	al, err := s.db.GetAssets(ctx)
	if err != nil {
		respond(w, r, s.log, errors.Wrap(err, "assets"))
		return
	}
	respond(w, r, s.log, al)
}

func (s *Server) GetTradesByChatID(w http.ResponseWriter, r *http.Request, chatID api.PChatID) {
	// TODO implement me
	panic("implement me")
}

func (s *Server) InitPersonalData(w http.ResponseWriter, r *http.Request, chatID api.PChatID) {
	ctx := r.Context()
	err := s.db.UpsertPerson(ctx, string(chatID))
	if err != nil {
		respond(w, r, s.log, errors.Wrap(err, "upsert person"))
		return
	}
	respond(w, r, s.log, []byte("{}"))
}

func (s Server) GetAllTrades(w http.ResponseWriter, r *http.Request, params api.GetAllTradesParams) {
	ctx := r.Context()

	offset, limit := 0, 1000
	if params.Offset != nil {
		offset = int(*params.Offset)
	}
	if params.Limit != nil {
		limit = int(*params.Limit)
	}

	pd, count, err := s.db.GetTrades(ctx, offset, limit, &repo.TradeFilter{Closed: params.TradeClosed.Bool()})
	if err != nil {
		respond(w, r, s.log, err)
		return
	}
	respond(w, r, s.log, api.TradesResp{
		Pagination: api.Pagination{
			Limit:  limit,
			Offset: offset,
			Total:  count,
		},
		Trades: pd,
	})
}

func (s Server) GetPersonalData(w http.ResponseWriter, r *http.Request, chatID api.PChatID) {
	ctx := r.Context()
	pd, err := s.db.GetPersonalData(ctx, string(chatID))
	if err != nil {
		respond(w, r, s.log, err)
		return
	}
	respond(w, r, s.log, pd)
}

func (s Server) UpdateState(w http.ResponseWriter, r *http.Request, chatID api.PChatID, params api.UpdateStateParams) {
	ctx := r.Context()
	err := s.db.UpdatePersonState(ctx, string(chatID), string(params.State))
	if err != nil {
		respond(w, r, s.log, err)
		return
	}
}

func (s Server) AddWallet(w http.ResponseWriter, r *http.Request, chatID api.PChatID, params api.AddWalletParams) {
	ctx := r.Context()
	err := s.db.UpsertPersonAddress(ctx, string(chatID), string(params.Wallet))
	if err != nil {
		respond(w, r, s.log, err)
		return
	}

	err = s.ass.UpdateAllBalances(ctx, ethgo.HexToAddress(string(params.Wallet)))
	if err != nil {
		s.log.WithError(err).Error("can't update balance of new wallet")
	}
}

func errHandler(w http.ResponseWriter, r *http.Request, err error) {
	if err == nil {
		return
	}

	header := http.StatusInternalServerError
	switch {
	case errors.Is(err, sql.ErrNoRows):
		header = http.StatusNotFound
	}
	w.WriteHeader(header)
	render.DefaultResponder(w, r, api.ErrorResp(err.Error()))
}

func respond(w http.ResponseWriter, r *http.Request, l *log.Logger, payload interface{}) {
	if err, ok := payload.(error); ok {
		l.WithError(err).Error()
		errHandler(w, r, err)
		return
	}
	if payload == nil {
		payload = []byte("{}")
	}
	render.DefaultResponder(w, r, payload)
}
