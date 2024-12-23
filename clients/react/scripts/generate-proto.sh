#!/bin/bash

# Create the output directory
mkdir -p src/generated

# Generate JavaScript code
protoc \
  --plugin=protoc-gen-ts=./node_modules/.bin/protoc-gen-ts \
  --ts_out=service=grpc-web:src/generated \
  --js_out=import_style=commonjs:src/generated \
  --grpc-web_out=import_style=typescript,mode=grpcwebtext:src/generated \
  -I ../../api/proto \
  ../../api/proto/dex/v1/dex.proto 