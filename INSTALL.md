# ローカルインストールガイド

## 方法1: go install を使用（推奨）

```bash
# リポジトリのルートディレクトリで実行
go install

# または、任意の場所から
go install github.com/neptaco/unity-cli@latest
```

これにより `$GOPATH/bin` または `$HOME/go/bin` にインストールされます。

## 方法2: Task を使用

```bash
# Taskfile に定義済みのインストールタスクを実行
task install
```

## 方法3: 手動でPATHに配置

```bash
# ビルド
task build
# または
go build -o unity-cli main.go

# /usr/local/bin にコピー（macOS/Linux）
sudo cp dist/unity-cli /usr/local/bin/

# または、ホームディレクトリのbinに配置
mkdir -p ~/bin
cp dist/unity-cli ~/bin/
# ~/.zshrc または ~/.bashrc に以下を追加
echo 'export PATH="$HOME/bin:$PATH"' >> ~/.zshrc
source ~/.zshrc
```

## 方法4: エイリアスとして設定

```bash
# ビルド
task build

# エイリアスを設定（~/.zshrc または ~/.bashrc に追加）
echo "alias unity-cli='$(pwd)/dist/unity-cli'" >> ~/.zshrc
source ~/.zshrc
```

## 方法5: シンボリックリンクを作成

```bash
# ビルド
task build

# シンボリックリンクを作成
sudo ln -sf $(pwd)/dist/unity-cli /usr/local/bin/unity-cli
```

## インストール確認

```bash
# インストールされたか確認
which unity-cli

# バージョン確認（まだ未実装のため、ヘルプを表示）
unity-cli --help

# 動作確認
unity-cli editor list
```

## アンインストール

### go install でインストールした場合
```bash
rm $(go env GOPATH)/bin/unity-cli
```

### /usr/local/bin にインストールした場合
```bash
sudo rm /usr/local/bin/unity-cli
```

### シンボリックリンクの場合
```bash
sudo rm /usr/local/bin/unity-cli
```

## 開発中の便利な使い方

開発中は、毎回ビルドせずに直接実行することも可能：

```bash
# Taskfile のdevタスクを使用
task dev -- editor list
task dev -- build --project ./test-project --target ios

# または直接go runを使用
go run main.go editor list
go run main.go build --project ./test-project --target android
```

## PATH の確認と設定

### 現在のPATHを確認
```bash
echo $PATH
```

### GOPATHを確認
```bash
go env GOPATH
```

### PATHに追加（永続化）

#### zsh (macOS デフォルト)
```bash
echo 'export PATH="$HOME/go/bin:$PATH"' >> ~/.zshrc
source ~/.zshrc
```

#### bash
```bash
echo 'export PATH="$HOME/go/bin:$PATH"' >> ~/.bashrc
source ~/.bashrc
```

## トラブルシューティング

### "command not found" エラー

1. PATHを確認
```bash
echo $PATH | tr ':' '\n' | grep -E "(go/bin|local/bin)"
```

2. インストール場所を確認
```bash
ls -la $(go env GOPATH)/bin/unity-cli
ls -la /usr/local/bin/unity-cli
```

3. PATHを更新
```bash
export PATH="$HOME/go/bin:$PATH"
```

### 権限エラー

```bash
# 実行権限を付与
chmod +x dist/unity-cli

# /usr/local/bin への書き込み権限
sudo cp dist/unity-cli /usr/local/bin/
```