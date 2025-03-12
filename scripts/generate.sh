#! /usr/bin/bash

set -x

cd metadata-editor
go run . refresh
go run . generate