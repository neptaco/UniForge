# Unity Changeset について

## Changesetとは？

Changeset（チェンジセット）は、Unity Editorの特定のビルドを一意に識別するためのハッシュ値です。Gitのコミットハッシュのようなもので、Unity Editorのソースコードの特定の状態を表します。

## なぜChangesetが必要？

Unity Hubは「公式リリースリスト」に含まれるバージョンのみを認識します。このリストには：
- 最新のLTSバージョン
- 最新のTechストリーム
- 一部のベータ版・アルファ版

しか含まれていません。

例えば：
- ✅ **2022.3.62f1** - リリースリストにある（最新のLTS）
- ❌ **2022.3.59f1** - リリースリストにない（古いパッチバージョン）
- ❌ **2022.3.10f1** - リリースリストにない（古いバージョン）

## Changesetの取得方法

### 方法1: Unity Download Archive
1. https://unity.com/releases/editor/archive にアクセス
2. 必要なバージョンを探す
3. "Release notes" または "Hub" ボタンをクリック
4. URLに含まれるchangesetを確認

例：`unityhub://2022.3.10f1/ff3792e53c62`
- Version: 2022.3.10f1
- Changeset: ff3792e53c62

### 方法2: インストール済みのUnityから確認
```bash
# macOS
/Applications/Unity/Hub/Editor/2022.3.10f1/Unity.app/Contents/MacOS/Unity -version

# 出力例:
# 2022.3.10f1 (ff3792e53c62)
```

### 方法3: Unity公式APIから取得（非公式）
Unity Download Assistantが使用するAPIエンドポイントから取得することも可能ですが、これは公式にサポートされていません。

## Changesetの挙動について

### 重要な発見
Unity Hubは、changesetパラメータが**指定されていれば**、その値の正確性を即座には検証しません：
- ✅ changesetあり（任意の値）→ ダウンロード開始
- ❌ changesetなし → 「リリースリストにない」エラー

ただし、**正しいchangesetを使用することを強く推奨**します。間違った値でもダウンロードは始まりますが、インストール完了後に問題が発生する可能性があります。

## Changesetを使用したインストール

Unity Hub CLIでは、リリースリストにないバージョンをインストールする際に`--changeset`パラメータが必要です：

```bash
# Unity Hub CLI直接使用
"/Applications/Unity Hub.app/Contents/MacOS/Unity Hub" -- --headless install \
  --version 2022.3.10f1 \
  --changeset ff3792e53c62

# unity-cli での実装（今後追加予定）
unity-cli editor install --version 2022.3.10f1 --changeset ff3792e53c62
```

## 実装への提案

1. **Changeset自動取得機能**
   - Unity Download ArchiveのAPIから自動取得
   - キャッシュして再利用

2. **エラーメッセージの改善**
   - リリースリストにない場合、changesetが必要な旨を表示
   - 利用可能なバージョンの提案

3. **バージョン検索機能**
   - 特定のバージョンのchangesetを検索
   - 近いバージョンの提案