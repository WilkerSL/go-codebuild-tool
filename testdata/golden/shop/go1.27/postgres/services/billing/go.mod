module example.com/shop/services/billing

go 1.27.0

require (
	example.com/shop/contracts v0.0.0
	github.com/jackc/pgx/v5 v5.11.0
	google.golang.org/grpc v1.84.0
	google.golang.org/protobuf v1.36.12
)

replace example.com/shop/contracts => ../../contracts
