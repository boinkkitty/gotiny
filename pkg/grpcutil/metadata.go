package grpcutil

import (
	"context"
	"strconv"

	"google.golang.org/grpc/metadata"
)

const UserIDKey = "x-user-id"

func UserIDFromContext(ctx context.Context) (int64, bool) {
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return 0, false
	}
	vals := md.Get(UserIDKey)
	if len(vals) == 0 {
		return 0, false
	}
	id, err := strconv.ParseInt(vals[0], 10, 64)
	if err != nil {
		return 0, false
	}
	return id, true
}

func ContextWithUserID(ctx context.Context, userID int64) context.Context {
	md := metadata.Pairs(UserIDKey, strconv.FormatInt(userID, 10))
	return metadata.NewOutgoingContext(ctx, md)
}
