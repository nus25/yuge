# Yuge
This is currently a personal project in development. 
Yuge currently works with [Gyoka](https://github.com/nus25/gyoka).

Yugeは、[Bluesky](https://bsky.app)のカスタムフィードを作成・管理するためのサーバーアプリケーションです。
(Yuge: 湯気 /jɯ̟ɡe/, steam form hot water)

## 特徴

- jetstream からポストを購読してフィードロジックに適合したポストをストアに追加します。
- configファイルまたはPDSの`app.bsky.feed.generator`レコードにフィードロジックを記述できます。
- 複数フィードを登録可能
- 永続状態の正本は SQLite で、Gyoka 反映は非同期 projection として扱います。
- Yuge単体でフィードジェネレーターとして動作（未実装）
執筆中

## インストール

subscriberのインストール
```bash
go install github.com/nus25/yuge/cmd/yuge_subscriber
```
## 使用方法
1. create directories
    ```bash
    # make config and data in work dir
    mkdir -p config data

    ```

2. create feed list in config dir:
    ```bash
    # create feed list file
    touch config/feedlist.yaml

    ```
    feedlist.yaml
    ```yaml
    feeds:
      - id: "feed1"
        uri: "at://did:plc:yourdid/app.bsky.feed.generator/feedrkey"
        configFile: "sample_feed_config.yaml"
    ```

3. create feed config

    フィード設定ファイルを指定した場合はconfigディレクトリ内に追加で作成。（PDSのapp.bsky.feed.generatorレコードからロードする場合はconfigFileを省略）

    sample_feed_config.yaml
    ```yaml
    #テスト用のコンフィグファイル
    logic:
      blocks:
        #ユーザーリスト(リスト内のユーザーを除外)
        - type: userlist
        options:
          listUri: at://did:plc:someuser/app.bsky.graph.list/somekey
          allow: false
        #リプライは除外
        - type: remove
          options:
            subject: item
            value: reply
        #langで日本語が設定されていないポストは除外
        - type: remove
          options:
            subject: language
            language: ja
            operator: '!='
        #正規表現フィルタ(200文字文字以上)
        - type: regex
          options:
            value: '^.{200,}$'
            invert: false 
            caseSensitive: false
        #連続投稿リミッター(10分以内に10投稿を上限とする)
        - type: limiter
          options:
            count: 10
            timeWindow: 10m
            cleanupFreq: 10m
    store:
      trimAt: 1200
      trimRemain: 1000
    detailedLog: false
    ```


4. サーバーの起動:

   ```bash
   bin/yuge_subscriber run --help
   ```
  migration/cutover が必要な場合のみ、起動時に以下の flag を使います。

  ```bash
  yuge_subscriber run --import-legacy-store-json
  yuge_subscriber run --import-legacy-store-json --import-legacy-store-enqueue-projection
  ```

  通常運用では import flag は不要です。

### Gyoka の認証

サービス間認証では、設定ディレクトリの `gyoka.yaml` に秘密鍵ファイルを指定します。

```yaml
directHost: https://gyoka-editor.example.com
audience: did:web:gyoka-editor.example.com#gyoka_editor
userIdentity: did:web:yuge-client.example.com
privateKeyFile: /run/secrets/gyoka-private-key
```

秘密鍵ファイルは Multibase 形式とし、実行ユーザーだけが読み取れる権限で管理してください。相対パスは `gyoka.yaml` のディレクトリを基準に解決します。コンテナではファイルを読み取り専用でマウントし、コンテナ内のパスを指定してください。鍵の変更には再起動が必要です。

`privateKeyFile` が指定されている場合は `GYOKA_APP_PASSWORD` より優先されます。読み込みや解析に失敗した場合、Gyoka projection は無効化され、subscriber は起動を続けます。`privateKeyFile` を指定しない場合は `GYOKA_APP_PASSWORD` を使えます。以前の `GYOKA_PRIVATE_KEY` は使用されないため、鍵ファイルへの移行が必要です。

### Jetstream カーソル

受信済みカーソルは `DATA_DIR`（`--data-directory-path`、既定値 `./data`）の
`jetstream-cursor.json` に約5秒ごとと切断時に保存され、再起動時に読み込まれます。
`--override-cursor` または `OVERRIDE_CURSOR` を明示すると保存値より優先されます。
0 以下を指定すると保存値を消去し、ライブストリームから開始します。
保存ファイルが破損している場合や読み込めない場合は起動に失敗します。

このカーソルはイベントの受信位置であり、フィードへの処理完了を保証しません。
停止時に処理途中のイベントが失われることや、保存間隔内のイベントが再処理されることがあります。
停止期間が Jetstream サーバーの保持期間を超えると、すべてのイベントを復元できるとは限りません。

## 設定オプション
執筆中


## CLI

自分のrepoのフィードレコードを操作するCLIツール

### 1. インストール

Goが入っている環境で以下のコマンドでインストール：

```bash
go install github.com/nus25/yuge/cmd/yuge_cli
```

### 2. 実行

```bash
yuge_cli
# または
$GOPATH/bin/yuge_cli
```

ヘルプの確認：

```bash
yuge_cli feed [command] -h
```

### 3. コマンド

#### フィード追加

```bash
yuge_cli feed publish \
  -i 自分のハンドル \
  --service-did "did:web:ジェネレーターのDID" \
  --display-name "フィード名" \
  --description "説明" \
  --avatar "data/icon.png" \
  myrecordkey
```

#### フィード削除

```bash
yuge_cli feed unpublish -i 自分のハンドル myrecordkey
```

#### フィードリスト

```bash
yuge_cli feed list [rkey または (*,?)入りのパターン]
```

### 環境変数の設定

以下のように環境変数を設定すれば、毎回のIDとアプリパスワードの入力を省略できます：

```bash
export BLUESKY_IDENTIFIER="nus.bsky.social"
export BLUESKY_PASSWORD=アプリパスワード
```


## ライセンス

MIT License

## 作者

[nus](https://bsky.app/profile/nus.bsky.social)
