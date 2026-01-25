---
name: commit-conventions
description: このプロジェクトのコミットメッセージ規約。コミット時に自動参照される。
user-invocable: false
---

# UniForge コミットメッセージ規約

このプロジェクトでは [Conventional Commits](https://www.conventionalcommits.org/) を採用しています。
GoReleaser が自動生成する CHANGELOG に反映されるため、適切なプレフィックスの選択が重要です。

## プレフィックス一覧

### ユーザー向け（CHANGELOGに含まれる）

| プレフィックス | 用途 | 例 |
|--------------|------|-----|
| `feat:` | **新機能** - ユーザーが使う新しいCLIコマンドや機能 | `feat: add test command for Unity Test Runner` |
| `fix:` | **バグ修正** - ユーザーが遭遇する問題の修正 | `fix: editor version parsing fails on Windows` |
| `BREAKING CHANGE:` | **破壊的変更** - 既存の動作が変わる変更 | `BREAKING CHANGE: rename run command to execute` |

### 内部向け（CHANGELOGに含まれない）

| プレフィックス | 用途 | 例 |
|--------------|------|-----|
| `ci:` | CI/CD設定、GitHub Actions、linter設定 | `ci: add golangci-lint to workflow` |
| `chore:` | 開発ツール、依存関係更新、内部改善 | `chore: add release workflow skills` |
| `docs:` | ドキュメントのみの変更 | `docs: update README installation guide` |
| `refactor:` | 機能変更を伴わないコード改善 | `refactor: extract validation logic` |
| `test:` | テストの追加・修正 | `test: add unit tests for meta checker` |
| `style:` | コードフォーマット（go fmt等） | `style: format code with go fmt` |
| `perf:` | パフォーマンス改善 | `perf: optimize file scanning` |
| `build:` | ビルドシステム、外部依存関係 | `build: update Go version to 1.24` |

## 判断基準

### feat: vs chore:

- **feat:** ユーザーが直接使う機能（CLIコマンド、オプション追加）
- **chore:** 開発者向けツール、CI設定、スキルファイル

### fix: vs ci:

- **fix:** ユーザーが遭遇するバグの修正
- **ci:** CI/CD パイプラインの問題修正、lintエラー修正

## 例

```
# Good - ユーザー向け機能
feat: add license management commands for Unity CI/CD

# Good - CI設定の変更
ci: use golangci-lint v2 in GitHub Actions

# Good - 内部ツール
chore: add release workflow skills for Claude Code

# Bad - 内部変更なのに feat を使用
feat: add release workflow skills for Claude Code
```

## スコープ（オプション）

必要に応じてスコープを追加できます：

```
feat(logs): add colorized output
fix(meta): handle symlinks correctly
ci(lint): upgrade to golangci-lint v2
```
