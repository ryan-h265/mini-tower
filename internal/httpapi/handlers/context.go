package handlers

import "context"

type contextKey string

const (
	ctxKeyTeamID      contextKey = "teamID"
	ctxKeyTeamSlug    contextKey = "teamSlug"
	ctxKeyTeamTokenID contextKey = "teamTokenID"
	ctxKeyTokenRole   contextKey = "tokenRole"
	ctxKeyRunnerID    contextKey = "runnerID"
	ctxKeyEnvironment contextKey = "environment"
)

func WithTeamID(ctx context.Context, teamID int64) context.Context {
	return context.WithValue(ctx, ctxKeyTeamID, teamID)
}

func teamIDFromContext(ctx context.Context) (int64, bool) {
	value := ctx.Value(ctxKeyTeamID)
	id, ok := value.(int64)
	return id, ok
}

func WithTeamSlug(ctx context.Context, slug string) context.Context {
	return context.WithValue(ctx, ctxKeyTeamSlug, slug)
}

func teamSlugFromContext(ctx context.Context) (string, bool) {
	value := ctx.Value(ctxKeyTeamSlug)
	slug, ok := value.(string)
	return slug, ok
}

func WithTeamTokenID(ctx context.Context, tokenID int64) context.Context {
	return context.WithValue(ctx, ctxKeyTeamTokenID, tokenID)
}

func teamTokenIDFromContext(ctx context.Context) (int64, bool) {
	value := ctx.Value(ctxKeyTeamTokenID)
	id, ok := value.(int64)
	return id, ok
}

func WithTokenRole(ctx context.Context, role string) context.Context {
	return context.WithValue(ctx, ctxKeyTokenRole, role)
}

func TokenRoleFromContext(ctx context.Context) (string, bool) {
	value := ctx.Value(ctxKeyTokenRole)
	role, ok := value.(string)
	return role, ok
}

func WithRunnerID(ctx context.Context, runnerID int64) context.Context {
	return context.WithValue(ctx, ctxKeyRunnerID, runnerID)
}

func runnerIDFromContext(ctx context.Context) (int64, bool) {
	value := ctx.Value(ctxKeyRunnerID)
	id, ok := value.(int64)
	return id, ok
}

func WithEnvironment(ctx context.Context, env string) context.Context {
	return context.WithValue(ctx, ctxKeyEnvironment, env)
}

func environmentFromContext(ctx context.Context) (string, bool) {
	value := ctx.Value(ctxKeyEnvironment)
	env, ok := value.(string)
	return env, ok
}
