<!--
SPDX-FileCopyrightText: (C) 2025 Intel Corporation
SPDX-License-Identifier: Apache-2.0
-->

# Alerts Monitor API

## Prerequisites

[OpenAPI Client and Code Generator](https://github.com/deepmap/oapi-codegen) was used to generate boilerplate API code.

## Code generation

Boilerplate code is generated from the openapi definition given in _api/v1/openapi.yaml_ using `oapi-codegen`,
and saved in _api/boilerplate_ folder. A new function was added to _Makefile_

```make
codegen:
	oapi-codegen -package api -generate types ./api/v1/openapi.yaml > ./api/boilerplate/types.gen.go
	oapi-codegen -package api -generate client ./api/v1/openapi.yaml > ./api/boilerplate/client.gen.go
	oapi-codegen -package api -generate server ./api/v1/openapi.yaml > ./api/boilerplate/server.gen.go
	oapi-codegen -package api -generate spec ./api/v1/openapi.yaml > ./api/boilerplate/spec.gen.go
```

To generate boilerplate use

```bash
make codegen
```

## File structure

- **api.go** - server startup
- **client.go** - client side API code, may be useful for functional tests
- **server.go** - API server code, including all endpoint handling functions
- **spec.go** - Swagger (to be expanded)
- **types.go** - data models
- **openapi.yaml** - OpenAPI definition of Alerts Monitor API

## API call handler functions

All API handlers can be found in _api/v1/server.go_.

A list of handler functions can be found in the `RegisterHandlersWithBaseURL` function. Each handler consists of two distinct parts, request parameters handling part and, for now, a generic 501 "Not Implemented" response status.

Example of a basic handler function with no additional functionality other than returning 501 and validating parameters:

```go
// PatchAlertReceiver converts echo context to params.
func (w *ServerInterfaceWrapper) PatchAlertReceiver(ctx echo.Context) error {
	var err error
	// ------------- Path parameter "receiverID" -------------
	var receiverID openapiTypes.UUID

	err = runtime.BindStyledParameterWithLocation("simple", false, "receiverID", runtime.ParamLocationPath, ctx.Param("receiverID"), &receiverID)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, fmt.Sprintf("Invalid format for parameter receiverID: %s", err))
	}

	// mock API response 
	return ctx.JSON(http.StatusNotImplemented, map[string]int{"status": http.StatusNotImplemented})

	// code below commented out to enable mock API response
	// Invoke the callback with all the unmarshaled arguments
	// err = w.Handler.PatchAlertReceiver(ctx, receiverID)
	// return err
}
```
