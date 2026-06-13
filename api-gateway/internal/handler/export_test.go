package handler

import "context"

func ContextWithUserIDForTest(ctx context.Context, id int64) context.Context {
	return contextWithUserID(ctx, id)
}

func UserIDFromContextForTest(ctx context.Context) (int64, bool) {
	return userIDFromContext(ctx)
}
