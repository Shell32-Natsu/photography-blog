#! /usr/bin/bash

set -x

cd metadata-editor
go run . refresh
go run . generate

# look up the size and EXIF data of new photos (photos.json) and build the
# site into public/ for a local check
cd ../..
go run ./cmd/photoblog
