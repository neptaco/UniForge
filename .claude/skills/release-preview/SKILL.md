---
name: release-preview
description: 次回リリースに含まれる変更をプレビューする。「リリースプレビュー」「次のリリース」「リリース内容確認」と言われた場合に使用する。
disable-model-invocation: true
---

# リリースプレビュー

次回リリースに含まれる変更をプレビューします。

## 現在の状態

```bash
# 現在のバージョン
echo "現在のタグ: $(git describe --tags --abbrev=0 2>/dev/null || echo 'なし')"

# ブランチ
echo "ブランチ: $(git branch --show-current)"
```

## 前回リリースからの変更

### コミット一覧

!`git log $(git describe --tags --abbrev=0 2>/dev/null || echo "HEAD~20")..HEAD --oneline --no-merges 2>/dev/null || git log --oneline -20`

### 変更の分類

リリースノートに含まれるもの：
- `feat:` 新機能
- `fix:` バグ修正
- `BREAKING CHANGE:` 破壊的変更

リリースノートに含まれないもの：
- `docs:`, `test:`, `chore:`, `ci:`, `build:`, `refactor:`, `style:`, `perf:`

## スナップショットビルド（ドライラン）

ローカルでビルドをテストする場合：

```bash
task release-snapshot
```

これにより `dist/` ディレクトリにビルド成果物が生成されます。

## 推奨バージョン

変更内容に基づいて、以下のバージョンを推奨：

| 変更タイプ | 次のバージョン |
|-----------|---------------|
| BREAKING CHANGE あり | メジャーアップ（例: v1.x.x → v2.0.0） |
| feat: あり | マイナーアップ（例: v1.0.x → v1.1.0） |
| fix: のみ | パッチアップ（例: v1.0.0 → v1.0.1） |

## 次のステップ

リリースを実行する場合：

```bash
/release v<VERSION>
```
