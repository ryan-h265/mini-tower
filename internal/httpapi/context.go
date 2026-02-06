package httpapi

import "context"

type contextKey string

const (
	ctxKeyTeamID      contextKey = "teamID"
	ctxKeyTeamTokenID contextKey = "teamTokenID"
)

func withTeamID(ctx context.Context, teamID int64) context.Context {
	return context.WithValue(ctx, ctxKeyTeamID, teamID)
}

func teamIDFromContext(ctx context.Context) (int64, bool) {
	value := ctx.Value(ctxKeyTeamID)
	id, ok := value.(int64)
	return id, ok
}

func withTeamTokenID(ctx context.Context, tokenID int64) context.Context {
	return context.WithValue(ctx, ctxKeyTeamTokenID, tokenID)
}

func teamTokenIDFromContext(ctx context.Context) (int64, bool) {
	value := ctx.Value(ctxKeyTeamTokenID)
	id, ok := value.(int64)
	return id, ok
}
