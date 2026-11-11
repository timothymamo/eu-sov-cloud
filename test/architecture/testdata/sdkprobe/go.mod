module example.com/sdkprobe

go 1.27.4

require (
	github.com/aws/aws-sdk-go-v2 v0.0.0
	github.com/aws/smithy-go v0.0.0 // indirect
)

replace github.com/aws/aws-sdk-go-v2 => ./sdk

replace github.com/aws/smithy-go => ./smithy
