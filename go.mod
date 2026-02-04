module github.com/everyday-items/hexagon

go 1.25.5

require (
	github.com/everyday-items/ai-core v0.0.0
	github.com/everyday-items/toolkit v0.0.0
	github.com/redis/go-redis/v9 v9.17.3
	golang.org/x/sync v0.19.0
	gopkg.in/yaml.v3 v3.0.1
)

require (
	github.com/cespare/xxhash/v2 v2.3.0 // indirect
	github.com/dgryski/go-rendezvous v0.0.0-20200823014737-9f7001d12a5f // indirect
	github.com/google/uuid v1.6.0 // indirect
)

// 本地开发时使用 replace 指向本地路径
replace (
	github.com/everyday-items/ai-core => ../ai-core
	github.com/everyday-items/toolkit => ../toolkit
)
