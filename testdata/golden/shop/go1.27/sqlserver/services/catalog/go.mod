module example.com/shop/services/catalog

go 1.27.0

require (
	example.com/shop/contracts v0.0.0
	example.com/shop/databases/shared v0.0.0
	github.com/microsoft/go-mssqldb v1.11.2
	google.golang.org/grpc v1.84.0
	google.golang.org/protobuf v1.36.12
)

replace example.com/shop/contracts => ../../contracts

replace example.com/shop/databases/shared => ../../databases/shared
