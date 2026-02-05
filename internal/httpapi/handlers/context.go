package handlers

import "context"

type contextKey string

const (
  ctxKeyTeamID        contextKey = "teamID"
  ctxKeyTeamTokenID   contextKey = "teamTokenID"
  ctxKeyRunnerID      contextKey = "runnerID"
  ctxKeyEnvironmentID contextKey = "environmentID"
)

func WithTeamID(ctx context.Context, teamID int64) context.Context {
  return context.WithValue(ctx, ctxKeyTeamID, teamID)
}

func teamIDFromContext(ctx context.Context) (int64, bool) {
  value := ctx.Value(ctxKeyTeamID)
  id, ok := value.(int64)
  return id, ok
}

func WithTeamTokenID(ctx context.Context, tokenID int64) context.Context {
  return context.WithValue(ctx, ctxKeyTeamTokenID, tokenID)
}

func teamTokenIDFromContext(ctx context.Context) (int64, bool) {
  value := ctx.Value(ctxKeyTeamTokenID)
  id, ok := value.(int64)
  return id, ok
}

func WithRunnerID(ctx context.Context, runnerID int64) context.Context {
  return context.WithValue(ctx, ctxKeyRunnerID, runnerID)
}

func runnerIDFromContext(ctx context.Context) (int64, bool) {
  value := ctx.Value(ctxKeyRunnerID)
  id, ok := value.(int64)
  return id, ok
}

func WithEnvironmentID(ctx context.Context, envID int64) context.Context {
  return context.WithValue(ctx, ctxKeyEnvironmentID, envID)
}

func environmentIDFromContext(ctx context.Context) (int64, bool) {
  value := ctx.Value(ctxKeyEnvironmentID)
  id, ok := value.(int64)
  return id, ok
}
