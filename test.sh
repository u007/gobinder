#!/bin/bash

cd docker && docker-compose up -d
cd ..
GO_ENV=test go test dgraph/*test.go
