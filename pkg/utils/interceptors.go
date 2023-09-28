// SPDX-License-Identifier: Apache-2.0
// Copyright (C) 2023 Intel Corporation
// Copyright (c) 2022-2023 Dell Inc, or its subsidiaries.

// Package utils contains utility functions
package utils

import (
	"context"
	"fmt"
	"log"

	"github.com/grpc-ecosystem/go-grpc-middleware/v2/interceptors/logging"
)

// InterceptorLogger creates logger for interceptors based on default Go logger
func InterceptorLogger(l *log.Logger) logging.Logger {
	return logging.LoggerFunc(func(_ context.Context, lvl logging.Level, msg string, fields ...any) {
		switch lvl {
		case logging.LevelDebug:
			msg = fmt.Sprintf("DEBUG :%v", msg)
		case logging.LevelInfo:
			msg = fmt.Sprintf("INFO :%v", msg)
		case logging.LevelWarn:
			msg = fmt.Sprintf("WARN :%v", msg)
		case logging.LevelError:
			msg = fmt.Sprintf("ERROR :%v", msg)
		default:
			panic(fmt.Sprintf("unknown level %v", lvl))
		}
		l.Println(append([]any{"msg", msg}, fields...))
	})
}
