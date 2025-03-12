#! /usr/bin/bash

set -x

PORT=18080

cd metadata-editor
go run . server --port $PORT
