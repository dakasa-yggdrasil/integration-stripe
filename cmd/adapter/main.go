package main

import (
	_ "github.com/dakasa-yggdrasil/yggdrasil-sdk-go/adapter"
	_ "github.com/dakasa-yggdrasil/yggdrasil-sdk-go/sig/hmac"
	_ "github.com/dakasa-yggdrasil/yggdrasil-sdk-go/webhookhttp"
	_ "github.com/stretchr/testify/require"
	_ "github.com/stripe/stripe-go/v83"
	_ "go.uber.org/zap"
)

func main() {}
