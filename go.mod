module github.com/halliday/go-values

go 1.18

replace github.com/halliday/go-errors => ../go-errors

replace github.com/halliday/go-module => ../go-module

replace github.com/halliday/go-tools => ../go-tools

require (
	github.com/halliday/go-errors v1.0.0
	github.com/halliday/go-module v1.0.0
	github.com/halliday/go-tools v1.0.0
)

require github.com/google/uuid v1.3.0 // indirect
