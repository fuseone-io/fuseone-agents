package httpapi

import (
	"context"
	"errors"
	"fmt"

	"github.com/fuseone/agents/internal/admin"
	"github.com/fuseone/agents/internal/domain"
	"github.com/fuseone/agents/internal/httpapi/openapi"
)

// Money is the installation currency configuration, declared by this layer.
type Money interface {
	Current(ctx context.Context) (admin.MoneyConfig, error)
	Set(ctx context.Context, by domain.UserID, scope domain.Scope, config admin.MoneyConfig) error
}

func (s *Server) WithMoney(money Money) *Server {
	s.money = money
	return s
}

func (s *Server) GetMoney(
	ctx context.Context, _ openapi.GetMoneyRequestObject,
) (openapi.GetMoneyResponseObject, error) {
	config, err := s.moneyOrDefault(ctx)
	if err != nil {
		return nil, err
	}
	return openapi.GetMoney200JSONResponse(moneyOut(config)), nil
}

func (s *Server) SetMoney(
	ctx context.Context, req openapi.SetMoneyRequestObject,
) (openapi.SetMoneyResponseObject, error) {
	if resp := s.refuse(ctx, domain.PermBudgetWrite); resp != nil {
		return openapi.SetMoney403ApplicationProblemPlusJSONResponse{
			ForbiddenApplicationProblemPlusJSONResponse: *resp,
		}, nil
	}
	if s.money == nil || req.Body == nil {
		return openapi.SetMoney204Response{}, nil
	}

	if err := s.money.Set(ctx, callerOf(ctx), adminScope, moneyIn(*req.Body)); err != nil {
		if errors.Is(err, admin.ErrMoneyInvalid) {
			return openapi.SetMoney400ApplicationProblemPlusJSONResponse{
				BadRequestApplicationProblemPlusJSONResponse: openapi.BadRequestApplicationProblemPlusJSONResponse(
					notStored(err.Error())),
			}, nil
		}
		return nil, fmt.Errorf("set money settings: %w", err)
	}
	return openapi.SetMoney204Response{}, nil
}

func (s *Server) moneyOrDefault(ctx context.Context) (admin.MoneyConfig, error) {
	if s.money == nil {
		return admin.DefaultMoney, nil
	}
	config, err := s.money.Current(ctx)
	if err != nil {
		return admin.MoneyConfig{}, fmt.Errorf("read money settings: %w", err)
	}
	return config, nil
}

func moneyOut(config admin.MoneyConfig) openapi.MoneySettings {
	return openapi.MoneySettings{Currency: config.Currency}
}

func moneyIn(body openapi.MoneySettings) admin.MoneyConfig {
	return admin.MoneyConfig{Currency: body.Currency}
}
