//go:build !darwin

package main

import "errors"

func dispatch([]string) error { return errors.New("the release canary helper runs only on macOS") }
