// SPDX-FileCopyrightText: (C) 2025 Intel Corporation
// SPDX-License-Identifier: Apache-2.0

package app

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
)

type RealmAccess struct {
	Roles []string `json:"roles"`
}

type JWTPayload struct {
	RealmAccess RealmAccess `json:"realm_access"`
}

var r = regexp.MustCompile(`^Bearer (\S+)$`)

func getB64JWT(authorizationHeader string) (string, error) {
	match := r.FindStringSubmatch(authorizationHeader)
	if len(match) != 2 {
		return "", errors.New("unable to extract token from authorization header")
	}
	return match[1], nil
}

func getRoles(jsonBytes []byte) ([]string, error) {
	var payload JWTPayload
	err := json.Unmarshal(jsonBytes, &payload)
	if err != nil {
		return nil, err
	}
	return payload.RealmAccess.Roles, nil
}

func restorePadding(input string) string {
	for len(input)%4 != 0 {
		input += "="
	}
	return input
}

func extractRolesFromJWT(jwt string) ([]string, error) {
	jwtSplit := strings.Split(jwt, ".")

	if len(jwtSplit) != 3 {
		return nil, errors.New("invalid token format")
	}

	payloadBytes, err := base64.StdEncoding.DecodeString(restorePadding(jwtSplit[1]))
	if err != nil {
		return nil, fmt.Errorf("unable to decode: %w", err)
	}

	roles, err := getRoles(payloadBytes)
	if err != nil {
		return nil, fmt.Errorf("unable to get roles: %w", err)
	}
	return roles, nil
}
