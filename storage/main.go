package main

import (
	"github.com/PlakarKorp/go-kloset-sdk"
	"github.com/PlakarKorp/integration-gcs"
)

func main() {
	sdk.RunStorage(gcs.NewStore)
}
