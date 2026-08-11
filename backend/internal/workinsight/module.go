package workinsight

import (
	"github.com/Wei-Shaw/sub2api/internal/securityaudit"
	"github.com/google/wire"
)

var ProviderSet = wire.NewSet(
	NewRepository,
	NewService,
	NewAdminHandler,
	wire.Bind(new(securityaudit.InsightSink), new(*Service)),
)
