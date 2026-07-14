# Test and Tool Matrix

Started automated baseline at 2026-07-14T22:08:00Z on branch security-audit-20260714.
All commands in this matrix were executed through WSL from /mnt/c/Users/geoff/Projects/nodex. Native Windows Git is used separately for Git operations; this script does not call Git.

## go version

```text
COMMAND: go version
START: 2026-07-14T21:58:24Z
EXIT: 0
DURATION_SECONDS: 0
LOG_SHA256: 291c1d3353834b3c813203a45898c409e56afa3824193034ba157453744f2633
```

## go env selected

```text
COMMAND: go env GOVERSION GOOS GOARCH GOPATH GOMOD GOTOOLCHAIN
START: 2026-07-14T21:58:24Z
EXIT: 0
DURATION_SECONDS: 0
LOG_SHA256: 8be1bbd0938e771bca52cb726812c2504b9b289aea4f10d9664f8440c2abc25f
```

## go mod verify

```text
COMMAND: go mod verify
START: 2026-07-14T21:58:24Z
EXIT: 0
DURATION_SECONDS: 1
LOG_SHA256: b4537ed75f533f993f371954de47e42a793b8e5b0587577de7e27fb3e50696bd
```

## go list modules json

```text
COMMAND: sh -c go list -m -json all > research/nodex-security-audit-20260714-215200/go-list-modules.json
START: 2026-07-14T21:58:25Z
EXIT: 0
DURATION_SECONDS: 0
LOG_SHA256: e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855
```

## go build all packages

```text
COMMAND: go build ./...
START: 2026-07-14T21:58:25Z
EXIT: 0
DURATION_SECONDS: 2
LOG_SHA256: e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855
```

## go test all packages

```text
COMMAND: go test -count=1 ./...
START: 2026-07-14T21:58:27Z
EXIT: 0
DURATION_SECONDS: 5
LOG_SHA256: 082b216d90e8a05a04cf75159e7e4bc62eeaf2d60dc6d72684f146a7027ee8ac
```

## go test race all packages

```text
COMMAND: go test -race -count=1 ./...
START: 2026-07-14T21:58:32Z
EXIT: 0
DURATION_SECONDS: 5
LOG_SHA256: c1cb62fce93ac3d7618b74581c940748e79e74c5701a76384b2cb9d0a79e259d
```

## go vet all packages

```text
COMMAND: go vet ./...
START: 2026-07-14T21:58:37Z
EXIT: 0
DURATION_SECONDS: 1
LOG_SHA256: e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855
```

## gofmt check

```text
COMMAND: sh -c diff=$(gofmt -s -d .); if [ -n "$diff" ]; then printf "%s\n" "$diff"; exit 1; fi
START: 2026-07-14T21:58:38Z
EXIT: 0
DURATION_SECONDS: 0
LOG_SHA256: e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855
```

## staticcheck all packages

```text
COMMAND: staticcheck ./...
START: 2026-07-14T21:58:39Z
EXIT: 0
DURATION_SECONDS: 2
LOG_SHA256: e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855
```

## govulncheck all packages

```text
COMMAND: govulncheck ./...
START: 2026-07-14T21:58:41Z
EXIT: 0
DURATION_SECONDS: 2
LOG_SHA256: 3016e51e4eac0d421674d2128bbbdefb2924b4646e0c14a1ab034977ad73fae5
```

## gosec all packages

```text
COMMAND: gosec ./...
START: 2026-07-14T21:58:44Z
EXIT: 0
DURATION_SECONDS: 3
LOG_SHA256: 6b92551ceda4ceb0e47a6be443f819ffd8d22de2edc06efbce3ff472b8edfc44
```

## gitleaks secret scan

```text
SKIPPED: gitleaks not installed; no privileged install attempted. Manual regex secret scan executed below.
```

## manual secret pattern scan

```text
COMMAND: sh -c rg -n --hidden --glob !.git --glob !research/nodex-security-audit-20260714-215200/tool-logs --glob !research/nodex-security-audit-20260714-215200/test-and-tool-matrix.md --glob !research/nodex-security-audit-20260714-215200/go-list-modules.json "(?i)(api[_-]?token|token_secret|password|passwd|authorization:|bearer [a-z0-9._=-]{16,}|pveapitoken|private key)" .
START: 2026-07-14T21:58:47Z
EXIT: 127
DURATION_SECONDS: 0
LOG_SHA256: 0d73eb1edc97dd23e0148db665d627f7e02c6885b22a79c63caa2ebc69f21ff7
```

## cross build linux amd64

```text
COMMAND: sh -c mkdir -p /tmp/nodex-audit-builds && GOOS=linux GOARCH=amd64 go build -o /tmp/nodex-audit-builds/nodex-linux-amd64 ./cmd/nodex
START: 2026-07-14T21:58:47Z
EXIT: 0
DURATION_SECONDS: 1
LOG_SHA256: e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855
```

## cross build linux arm64

```text
COMMAND: sh -c mkdir -p /tmp/nodex-audit-builds && GOOS=linux GOARCH=arm64 go build -o /tmp/nodex-audit-builds/nodex-linux-arm64 ./cmd/nodex
START: 2026-07-14T21:58:48Z
EXIT: 0
DURATION_SECONDS: 1
LOG_SHA256: e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855
```

## cross build darwin arm64

```text
COMMAND: sh -c mkdir -p /tmp/nodex-audit-builds && GOOS=darwin GOARCH=arm64 go build -o /tmp/nodex-audit-builds/nodex-darwin-arm64 ./cmd/nodex
START: 2026-07-14T21:58:49Z
EXIT: 0
DURATION_SECONDS: 2
LOG_SHA256: e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855
```

## cross build windows amd64

```text
COMMAND: sh -c mkdir -p /tmp/nodex-audit-builds && GOOS=windows GOARCH=amd64 go build -o /tmp/nodex-audit-builds/nodex-windows-amd64.exe ./cmd/nodex
START: 2026-07-14T21:58:51Z
EXIT: 0
DURATION_SECONDS: 2
LOG_SHA256: e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855
```

## bounded fuzz cli

```text
COMMAND: go test ./internal/cli -run ^$ -fuzz=Fuzz -fuzztime=10s
START: 2026-07-14T21:58:53Z
EXIT: 1
DURATION_SECONDS: 2
LOG_SHA256: 962b8086c4086323c43df5fb2f6f84c2aa00c49df29742b7e000c30025c6770a
```

## bounded fuzz config

```text
COMMAND: go test ./internal/config -run ^$ -fuzz=Fuzz -fuzztime=10s
START: 2026-07-14T21:58:55Z
EXIT: 1
DURATION_SECONDS: 1
LOG_SHA256: 5f24f48ddbd40631f6c94b3e567a90174a73ab1bb13576304d2719ab872835b0
```

## bounded fuzz credentials

```text
COMMAND: go test ./internal/credentials -run ^$ -fuzz=Fuzz -fuzztime=10s
START: 2026-07-14T21:58:56Z
EXIT: 1
DURATION_SECONDS: 0
LOG_SHA256: cf38ff061f435ac3fb13884043103868f98bc2573e11bebd80e57dbb0b0115e9
```

## bounded fuzz task

```text
COMMAND: go test ./internal/task -run ^$ -fuzz=Fuzz -fuzztime=10s
START: 2026-07-14T21:58:56Z
EXIT: 0
DURATION_SECONDS: 11
LOG_SHA256: fee4bcf47fab803f29be1d03a9640a39c30c21ff58ce77062f5de665dd75071b
```

## manual secret pattern scan python

```text
COMMAND: python3 -
START: 2026-07-14T21:59:58Z
EXIT: 0
DURATION_SECONDS: 0
LOG_SHA256: d46ea579266c7744ef9d003273390bc3573727ba519fe0e6dd8d564cbb41fa2b
```

## install gitleaks v8.28.0

```text
COMMAND: go install github.com/gitleaks/gitleaks/v8@v8.28.0
START: 2026-07-14T21:59:58Z
EXIT: 1
DURATION_SECONDS: 2
LOG_SHA256: 1558b50bef6acb8f0d6be3c211817ae1048ff3f4a606b85513ebf7763231d327
```

## gitleaks dir no git after install

```text
SKIPPED: gitleaks remained unavailable after user-local go install attempt; see install log.
```

## bounded fuzz cli ParseNodeVMID

```text
COMMAND: go test ./internal/cli -run ^$ -fuzz=FuzzParseNodeVMID -fuzztime=10s
START: 2026-07-14T22:00:00Z
EXIT: 0
DURATION_SECONDS: 12
LOG_SHA256: 1db7e629d29a1ff7593e08d13d3ceedd99b134887cc841103b71b715cdc23e5a
```

## bounded fuzz cli ParseKeyValueArgs

```text
COMMAND: go test ./internal/cli -run ^$ -fuzz=FuzzParseKeyValueArgs -fuzztime=10s
START: 2026-07-14T22:00:12Z
EXIT: 0
DURATION_SECONDS: 12
LOG_SHA256: c4cd372e2c15caa9228730eaecd8eca05fb43583463538bf8acfc233192dcef0
```

## bounded fuzz config ValidateEndpoint

```text
COMMAND: go test ./internal/config -run ^$ -fuzz=FuzzValidateEndpoint -fuzztime=10s
START: 2026-07-14T22:00:24Z
EXIT: 0
DURATION_SECONDS: 11
LOG_SHA256: 8a2c40ebeb7c8e5263210a600eb8b8011d75b6b0bebc127d34523c7a6deb9f5d
```

## bounded fuzz config ProfileNameValidate

```text
COMMAND: go test ./internal/config -run ^$ -fuzz=FuzzProfileNameValidate -fuzztime=10s
START: 2026-07-14T22:00:35Z
EXIT: 0
DURATION_SECONDS: 11
LOG_SHA256: c4efe6d083e48fa39abf504c4975a350679259b2cdadff88a4df930bd875041a
```

## bounded fuzz credentials ParseCredentialRefStrict

```text
COMMAND: go test ./internal/credentials -run ^$ -fuzz=FuzzParseCredentialRefStrict -fuzztime=10s
START: 2026-07-14T22:00:46Z
EXIT: 0
DURATION_SECONDS: 11
LOG_SHA256: 0b1b13053f6acd59905e88d56cc6f703b754380ebd80eada13c123f92213b1c1
```

## bounded fuzz credentials ValidateName

```text
COMMAND: go test ./internal/credentials -run ^$ -fuzz=FuzzValidateName -fuzztime=10s
START: 2026-07-14T22:00:57Z
EXIT: 0
DURATION_SECONDS: 12
LOG_SHA256: 48560a4108499299f726eb1f0c75bda76dcbe3b6ef022d434be0fb2179bf4c89
```

## install gitleaks zricethezav v8.28.0

```text
COMMAND: go install github.com/zricethezav/gitleaks/v8@v8.28.0
START: 2026-07-14T22:01:52Z
EXIT: 0
DURATION_SECONDS: 10
LOG_SHA256: feffea2c21baa0f0c22b5e2a67507f5b6edf4fca79585217a860b11eccaf7c8d
```

## gitleaks dir no git after zricethezav install

```text
COMMAND: gitleaks dir --no-git --redact --verbose .
START: 2026-07-14T22:02:02Z
EXIT: 126
DURATION_SECONDS: 0
LOG_SHA256: 3260eb044f7424f2ef214ed6d1f2347a50b616a0b7824aacc80801a1657f2592
```

## install gitleaks zricethezav v8.28.0

```text
COMMAND: go install github.com/zricethezav/gitleaks/v8@v8.28.0
START: 2026-07-14T22:02:33Z
EXIT: 0
DURATION_SECONDS: 1
LOG_SHA256: e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855
```

## gitleaks dir no git after zricethezav install

```text
COMMAND: gitleaks dir --no-git --redact --verbose .
START: 2026-07-14T22:02:34Z
EXIT: 126
DURATION_SECONDS: 0
LOG_SHA256: 3260eb044f7424f2ef214ed6d1f2347a50b616a0b7824aacc80801a1657f2592
```

## gitleaks dir redacted exit0

```text
COMMAND: gitleaks dir --redact --verbose --exit-code 0 .
START: 2026-07-14T22:02:34Z
EXIT: 0
DURATION_SECONDS: 0
LOG_SHA256: e4cd50d7bdf53e4339053f0d756ce3075d84a5aa4d349a97cee379b4b4cf6b3c
```

## independent verification: coverage validator

```text
COMMAND: python3 research/nodex-security-audit-20260714-215200/check-coverage.py
START: 2026-07-14T22:15:37Z
EXIT: 0
DURATION_SECONDS: 0
LOG_SHA256: 8e4575e3413eb9a2c3650425bd63f541f23cc1daff1d9e319cb9b63f3167ace4
```

## independent verification: operation inventory regenerate

```text
COMMAND: python3 research/nodex-security-audit-20260714-215200/generate_operation_inventory.py
START: 2026-07-14T22:15:37Z
EXIT: 0
DURATION_SECONDS: 0
LOG_SHA256: ff1efbcbfe1f77cb1d3e128e8ae0f58d4b9ae11aa10f57fcc86dd22bf211e974
```

## independent verification: focused remediation tests

```text
COMMAND: go test -count=1 ./internal/cli ./internal/output ./internal/redact ./internal/transport/httpclient
START: 2026-07-14T22:15:37Z
EXIT: 0
DURATION_SECONDS: 4
LOG_SHA256: 1906f2856a8c3a943f7fa53b47be9d2128618ce830cc5ae692cf1dc3a5712d42
```

## independent verification: go mod verify

```text
COMMAND: go mod verify
START: 2026-07-14T22:15:41Z
EXIT: 0
DURATION_SECONDS: 0
LOG_SHA256: b4537ed75f533f993f371954de47e42a793b8e5b0587577de7e27fb3e50696bd
```

## independent verification: go build all packages

```text
COMMAND: go build ./...
START: 2026-07-14T22:15:41Z
EXIT: 0
DURATION_SECONDS: 2
LOG_SHA256: e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855
```

## independent verification: go test all packages

```text
COMMAND: go test -count=1 ./...
START: 2026-07-14T22:15:43Z
EXIT: 0
DURATION_SECONDS: 4
LOG_SHA256: f1365903c197f316d3f2f3b0af39030892396d27c0f620e77558113e39a250df
```

## independent verification: go test race all packages

```text
COMMAND: go test -race -count=1 ./...
START: 2026-07-14T22:15:47Z
EXIT: 0
DURATION_SECONDS: 7
LOG_SHA256: 817e5ff01b9207e378fb2e2028708f777df2d366d7308b960d4e137571f6f58e
```

## independent verification: go vet all packages

```text
COMMAND: go vet ./...
START: 2026-07-14T22:15:54Z
EXIT: 0
DURATION_SECONDS: 1
LOG_SHA256: e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855
```

## independent verification: gofmt check

```text
COMMAND: sh -c diff=$(gofmt -s -d .); if [ -n "$diff" ]; then printf "%s\n" "$diff"; exit 1; fi
START: 2026-07-14T22:15:55Z
EXIT: 0
DURATION_SECONDS: 1
LOG_SHA256: e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855
```

## independent verification: staticcheck all packages

```text
COMMAND: staticcheck ./...
START: 2026-07-14T22:15:56Z
EXIT: 0
DURATION_SECONDS: 2
LOG_SHA256: e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855
```

## independent verification: govulncheck all packages

```text
COMMAND: govulncheck ./...
START: 2026-07-14T22:15:58Z
EXIT: 0
DURATION_SECONDS: 3
LOG_SHA256: 3016e51e4eac0d421674d2128bbbdefb2924b4646e0c14a1ab034977ad73fae5
```

## independent verification: gosec all packages

```text
COMMAND: gosec ./...
START: 2026-07-14T22:16:01Z
EXIT: 0
DURATION_SECONDS: 3
LOG_SHA256: 872473a092a220d36de5ecbd66ed6a467bb1b7736749d8021ba1ead3cdb7c333
```

## independent verification: gitleaks dir redacted

```text
COMMAND: gitleaks dir --redact --verbose .
START: 2026-07-14T22:16:04Z
EXIT: 0
DURATION_SECONDS: 0
LOG_SHA256: a2179cace9765454266cb93529178362b19b77dbdeb57d193a37d291688db9d0
```

## independent verification: cross build linux amd64

```text
COMMAND: sh -c mkdir -p /tmp/nodex-audit-builds && GOOS=linux GOARCH=amd64 go build -o /tmp/nodex-audit-builds/nodex-linux-amd64 ./cmd/nodex
START: 2026-07-14T22:16:04Z
EXIT: 0
DURATION_SECONDS: 1
LOG_SHA256: e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855
```

## independent verification: cross build linux arm64

```text
COMMAND: sh -c mkdir -p /tmp/nodex-audit-builds && GOOS=linux GOARCH=arm64 go build -o /tmp/nodex-audit-builds/nodex-linux-arm64 ./cmd/nodex
START: 2026-07-14T22:16:05Z
EXIT: 0
DURATION_SECONDS: 2
LOG_SHA256: e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855
```

## independent verification: cross build darwin arm64

```text
COMMAND: sh -c mkdir -p /tmp/nodex-audit-builds && GOOS=darwin GOARCH=arm64 go build -o /tmp/nodex-audit-builds/nodex-darwin-arm64 ./cmd/nodex
START: 2026-07-14T22:16:07Z
EXIT: 0
DURATION_SECONDS: 2
LOG_SHA256: e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855
```

## independent verification: cross build windows amd64

```text
COMMAND: sh -c mkdir -p /tmp/nodex-audit-builds && GOOS=windows GOARCH=amd64 go build -o /tmp/nodex-audit-builds/nodex-windows-amd64.exe ./cmd/nodex
START: 2026-07-14T22:16:09Z
EXIT: 0
DURATION_SECONDS: 1
LOG_SHA256: e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855
```

## independent verification: bounded fuzz cli ParseNodeVMID

```text
COMMAND: go test ./internal/cli -run ^$ -fuzz=FuzzParseNodeVMID -fuzztime=10s
START: 2026-07-14T22:16:10Z
EXIT: 0
DURATION_SECONDS: 14
LOG_SHA256: ecde784daff4fdf9325edc1cbc1cfbc88bb8c77e64c08d89bd66e69079b23748
```

## independent verification: bounded fuzz cli ParseKeyValueArgs

```text
COMMAND: go test ./internal/cli -run ^$ -fuzz=FuzzParseKeyValueArgs -fuzztime=10s
START: 2026-07-14T22:16:24Z
EXIT: 0
DURATION_SECONDS: 11
LOG_SHA256: cd3ebf613f8ddf7ba461277bb4e41fc70bc8bac2740a93f0918f511882901963
```

## independent verification: bounded fuzz config ValidateEndpoint

```text
COMMAND: go test ./internal/config -run ^$ -fuzz=FuzzValidateEndpoint -fuzztime=10s
START: 2026-07-14T22:16:35Z
EXIT: 0
DURATION_SECONDS: 12
LOG_SHA256: 7c3a64f3601bf5a7659597a729408641e91a104f5c747d1c4573018449fa3a56
```

## independent verification: bounded fuzz config ProfileNameValidate

```text
COMMAND: go test ./internal/config -run ^$ -fuzz=FuzzProfileNameValidate -fuzztime=10s
START: 2026-07-14T22:16:47Z
EXIT: 0
DURATION_SECONDS: 10
LOG_SHA256: 7823bccaa8a2d2fe8825c8af95c5686211371acf68ea0df7c151a6fbcd637afc
```

## independent verification: bounded fuzz credentials ParseCredentialRefStrict

```text
COMMAND: go test ./internal/credentials -run ^$ -fuzz=FuzzParseCredentialRefStrict -fuzztime=10s
START: 2026-07-14T22:16:57Z
EXIT: 0
DURATION_SECONDS: 12
LOG_SHA256: ab37933804c4c4e95c82a6afb3ba38cbd8934955bc497e2f935f4b9f692c7d2b
```

## independent verification: bounded fuzz credentials ValidateName

```text
COMMAND: go test ./internal/credentials -run ^$ -fuzz=FuzzValidateName -fuzztime=10s
START: 2026-07-14T22:17:09Z
EXIT: 0
DURATION_SECONDS: 11
LOG_SHA256: e56671df4af8c7626326ddc4226ef567c5f40d5176106958079d1c911e52527b
```

## independent verification: bounded fuzz task

```text
COMMAND: go test ./internal/task -run ^$ -fuzz=Fuzz -fuzztime=10s
START: 2026-07-14T22:17:20Z
EXIT: 0
DURATION_SECONDS: 11
LOG_SHA256: 24e8403390b446ca17053b04a1c2512c4210daf4fa33ab1ab9738440fdd84723
```

## independent verification: staticcheck pinned v0.6.1

```text
COMMAND: go run honnef.co/go/tools/cmd/staticcheck@v0.6.1 ./...
START: 2026-07-14T22:19:28Z
EXIT: 0
DURATION_SECONDS: 7
LOG_SHA256: 2d69c1060ca7494c2d9cd01fd0e1eb4967b68df3430c3f920e062b6335d62875
```
