# mermd サンプルドキュメント

`mermd` は Markdown を HTML ライクに表示しつつ、**Mermaid** ダイアグラムを正しく描画するための CLI プレビューアです。

## 1. フローチャート (Flowchart)

```mermaid
flowchart LR
    A[Markdown ファイル] --> B[mermd CLI]
    B --> C{出力モード}
    C -->|ブラウザ| D[ローカルHTTPサーバー + ライブリロード]
    C -->|--html| E[HTML標準出力]
    D --> F[Mermaid.js で図を描画]
```

## 2. シーケンス図 (Sequence Diagram)

```mermaid
sequenceDiagram
    participant U as User
    participant CLI as mermd
    participant B as Browser
    U->>CLI: mermd sample.md
    CLI->>B: http://127.0.0.1:xxxx を開く
    B->>CLI: GET / & /events (SSE)
    U->>CLI: sample.md を編集・保存
    CLI-->>B: event: reload
    B->>CLI: GET /api/content
    B-->>U: 図とHTMLを即座に更新
```

## 3. GFM テーブル & タスクリスト

| 機能 | ステータス | 備考 |
| --- | --- | --- |
| GFM テーブル | 対応済み | `goldmark` 使用 |
| Mermaid 描画 | 対応済み | Light / Dark テーマ追従 |
| ライブリロード | 対応済み | SSE で差分更新 |

- [x] CLI 引数・標準入力対応
- [x] ダークモード切り替え
- [ ] お好みで機能追加
