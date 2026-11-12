module example.com/cmdprobe

go 1.27.4

require (
	example.com/cmdprobe/nested v0.0.0
	example.com/logging v0.0.0
	github.com/aws/aws-sdk-go-v2 v0.0.0
)

replace example.com/logging => ./logging

replace github.com/aws/aws-sdk-go-v2 => ./sdk

replace example.com/cmdprobe/nested => ./nested
