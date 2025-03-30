# Yuge
This is currently a personal project in development. Currently not working standalone.

Yugeは、[Bluesky](https://bsky.app)のカスタムフィードを作成・管理するためのサーバーアプリケーションです。
(Yuge: 湯気 /jɯ̟ɡe/, steam form hot water)

## 特徴

- jetstream からポストを購読してフィードロジックに適合したポストをストアに追加します。
- configファイルまたはPDSの`app.bsky.feed.generator`レコードにフィードロジックを記述できます。
- 複数フィードを登録可能
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
   執筆中

## 設定オプション
執筆中

## ライセンス

MIT License

## 作者

[nus](https://bsky.app/profile/nus.bsky.social)
