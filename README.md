# asagao

監視 SaaS（Datadog / Grafana / Mackerel / New Relic）のアラートを起点に、AI が
コードベースを調査して修正案を **PR として自動作成** するエージェント。

オンコール対応の初動（原因箇所の特定 → 修正案の用意）を自動化し、人間は PR レビューに集中できる状態を目指す。

## 仕組み

```
[Datadog/Grafana Webhook] ─┐
                           ├─→ provider 層 ─→ NormalizedAlert
[CLI: asagao run <file>]  ─┘                        │
                                                    ▼
                                Claude Managed Agents (CMA)
                          ┌────────────────────────────────────┐
                          │ Agent(一度作成) + Session(アラート毎)  │
                          │  repo を mount → 調査・修正 → push    │
                          │  → GitHub MCP で create_pull_request │
                          └────────────────────────────────────┘
```

- コードの探索・修正・PR 作成は **すべて Anthropic 側のコンテナ内**（CMA）で行われる。
  本ツールはオーケストレーション（アラート正規化 → セッション起動 → イベント消費）に専念する。
- 監視 SaaS の追加は `internal/provider` に `Provider` インターフェースを実装するだけ。
  MVP では **Datadog / Grafana** を実装、Mackerel / New Relic は空枠（TODO）。

## 必要なもの

| 種類 | 内容 |
| --- | --- |
| `ANTHROPIC_API_KEY` (env) | Anthropic API キー。`ant auth login` プロファイルでも可。 |
| `GITHUB_TOKEN` (env) | 対象 repo の clone/push 用 GitHub PAT（`Contents: Read and write`）。 |
| vault (事前作成) | **GitHub MCP サーバ用の OAuth 資格情報** を保持する vault。PR 作成に必須。clone 用 PAT とは別物。 |

### vault の用意（PR 作成に必須）

PR 作成は GitHub MCP サーバ（`https://api.githubcopilot.com/mcp/`）経由で行う。その認証情報は
CMA の **vault** に格納し、セッションに `vault_ids` で渡す。`ant` CLI などで事前に作成しておく:

```sh
# 例（詳細は Anthropic のドキュメント参照）
ant beta:vaults create --display-name "asagao-github"
ant beta:vaults:credentials create --vault-id vlt_xxxx ...   # GitHub MCP の OAuth 資格情報
```

作成した vault ID を `config.yaml` の `anthropic.vault_ids` に設定する。

## セットアップ

```sh
cp config.example.yaml config.yaml
# config.yaml を編集（repos マッピング、vault_ids など）

export ANTHROPIC_API_KEY=sk-ant-...
export GITHUB_TOKEN=ghp_...

go build ./...
```

初回起動時、Agent と Environment が自動作成され、ID がログに出力される。
**それらを `config.yaml` に書き戻す**ことで次回以降は再利用される（毎回作り直さない）。

## 使い方

### Webhook サーバとして常駐

```sh
go run ./cmd/asagao serve --config config.yaml
```

各 SaaS の通知先を `http://<host>:8080/webhook/<provider>` に向ける:

- Datadog: `.../webhook/datadog`
- Grafana: `.../webhook/grafana`

受信すると即 `202 Accepted` を返し、CMA セッションは非同期で走る（結果はログに出力）。
ヘルスチェックは `GET /healthz`。

### CLI で 1 件処理（E2E 検証・デバッグ用）

```sh
go run ./cmd/asagao run --config config.yaml --provider grafana --file alert.json
```

## 設定

`config.example.yaml` を参照。要点:

- シークレットは **環境変数のみ**（YAML には置かない）。
- `repos`: サービス名 → repo のマッピング。アラートの `service` タグから解決。
  `default` キーが未マッチ時のフォールバック。
- `anthropic.agent_id` / `agent_version` / `environment_id`: 初回自動作成 → 書き戻して再利用。

## 開発

```sh
go build ./...
go vet ./...
go test ./...
```

provider のパーサは `internal/provider/*_test.go`（テーブル駆動）でカバーしている。
`internal/provider/testdata/` に代表ペイロードのサンプルがある。

## MVP の非ゴール

- リトライ/バックオフ、メトリクス取得 API 連携、アラート重複排除、Webhook 署名検証は未実装（後続）。
- Mackerel / New Relic provider はインターフェース実装の空枠 + TODO のみ。
