# ローカルテストガイド

## 1. ビルド

```bash
# Taskを使用してビルド
task build

# または直接ビルド
go build -o dist/uniforge main.go
```

## 2. テスト用プロジェクトのセットアップ

```bash
# テスト用のモックUnityプロジェクトを作成
./scripts/test-setup.sh
```

## 3. 基本的なコマンドのテスト

### Unity Editor管理

```bash
# インストール済みのUnity Editorを確認
./dist/uniforge editor list

# Unity Editorのインストール（Unity Hubが必要）
./dist/uniforge editor install --version 2022.3.10f1

# プロジェクトから自動検出してインストール
./dist/uniforge editor install --from-project ./test-project
```

### ビルドコマンド（ドライラン）

```bash
# 実際のUnityプロジェクトがある場合
./dist/uniforge build \
  --project /path/to/your/unity/project \
  --target ios \
  --output ./Build/iOS \
  --log-file ./build.log

# カスタムビルドメソッドを使用
./dist/uniforge build \
  --project ./test-project \
  --target android \
  --method MyCompany.Builder.BuildAndroid \
  --args "config=debug,optimize=false"
```

### 実行コマンド

```bash
# Unity Editorでメソッドを実行
./dist/uniforge run \
  --project ./test-project \
  --execute-method TestRunner.RunTests \
  --quit

# バッチモードを無効にして対話的に実行
./dist/uniforge run \
  --project ./test-project \
  --batch-mode=false \
  --no-graphics=false
```

## 4. ユニットテストの実行

```bash
# 全テストを実行
task test

# 特定のパッケージのテストのみ実行
go test ./pkg/unity
go test ./pkg/platform
go test ./pkg/logger

# カバレッジレポート付きでテスト
task test-coverage
# coverage.htmlがブラウザで開きます

# 詳細なテスト出力
go test -v ./...

# レースコンディション検出付き
go test -race ./...
```

## 5. デバッグモード

```bash
# デバッグログを有効にして実行
./dist/uniforge --log-level debug editor list

# 環境変数でログレベルを設定
UNIFORGE_LOG_LEVEL=debug ./dist/uniforge build --project ./test-project --target ios

# カラー出力を無効化
./dist/uniforge --no-color editor list
```

## 6. 設定ファイルのテスト

```bash
# 設定ファイルを作成
cat > ~/.uniforge.yaml << EOF
log-level: debug
no-color: false
EOF

# 設定ファイルを使用して実行
./dist/uniforge editor list

# 別の設定ファイルを指定
./dist/uniforge --config ./my-config.yaml editor list
```

## 7. 環境変数のテスト

```bash
# Unity Hub パスを指定
UNIFORGE_HUB_PATH="/Applications/Unity Hub.app/Contents/MacOS/Unity Hub" \
  ./dist/uniforge editor list

# ログレベルを環境変数で設定
UNIFORGE_LOG_LEVEL=debug ./dist/uniforge editor list

# タイムアウトを設定（秒）
UNIFORGE_TIMEOUT=1800 ./dist/uniforge build --project ./test-project --target android
```

## 8. CI/CDモードのテスト

```bash
# CI向け出力フォーマットでビルド
./dist/uniforge build \
  --project ./test-project \
  --target windows \
  --ci-mode \
  --fail-on-warning

# GitHub Actions形式の出力を確認
./dist/uniforge build \
  --project ./test-project \
  --target ios \
  --ci-mode \
  --log-file - | grep "::error::" 
```

## 9. エラーケースのテスト

```bash
# 存在しないプロジェクト
./dist/uniforge build --project ./non-existent --target ios

# 無効なバージョン
./dist/uniforge editor install --version invalid-version

# Unity Hubが見つからない場合
UNIFORGE_HUB_PATH="/invalid/path" ./dist/uniforge editor list
```

## 10. パフォーマンステスト

```bash
# ビルド時間の計測
time ./dist/uniforge build --project ./test-project --target android

# メモリ使用量の確認（macOS）
/usr/bin/time -l ./dist/uniforge editor list

# メモリ使用量の確認（Linux）
/usr/bin/time -v ./dist/uniforge editor list
```

## トラブルシューティング

### Unity Hubが見つからない場合

```bash
# Unity Hubのパスを確認
ls -la "/Applications/Unity Hub.app/Contents/MacOS/Unity Hub"

# 環境変数で指定
export UNIFORGE_HUB_PATH="/Applications/Unity Hub.app/Contents/MacOS/Unity Hub"
```

### ビルドエラーの詳細確認

```bash
# 詳細ログでビルド
./dist/uniforge --log-level debug build \
  --project ./test-project \
  --target ios \
  --log-file ./detailed.log
```

### テストが失敗する場合

```bash
# 特定のテストのみ実行
go test -v -run TestLoadProject ./pkg/unity

# テストのタイムアウトを延長
go test -timeout 30s ./...
```