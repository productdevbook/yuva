module github.com/productdevbook/yuva/examples/draft-bot

go 1.27.1

require github.com/productdevbook/yuva/sdk/go v0.0.0

require (
	github.com/apapsch/go-jsonmerge/v2 v2.0.0 // indirect
	github.com/google/uuid v1.6.0 // indirect
	github.com/oapi-codegen/nullable v1.2.0 // indirect
	github.com/oapi-codegen/runtime v1.7.0 // indirect
)

replace github.com/productdevbook/yuva/sdk/go => ../../sdk/go
