gentoo-zh overlay bumpbot ，也可用于其他 overlay（大概）

使用方法:

```
GITHUB_REPOSITORY="microcai/gentoo-zh" GITHUB_TOKEN="ghp_xxxx" ./bumpbot --file overlay.toml --name "app-editors/cursor" --newver "7.36.11" --oldver ""
```


`name` `newver` `oldver` 来自 `nvcmp --file overlay.toml --json --newer` 

`github_account` 来自 `--file overlay.toml`



功能:
- [x] 读取 nvcmp 传递过来的 json，由 bumpbot 创建 bump issues

示例:
``` bash
$ nvchecker --file overlay.toml --keyfile keyfile.toml
$ nvcmp --file overlay.toml --json --newer | jq 'map({name, newver, oldver})' | jq -c
[{"name":"app-editors/cursor","newver":"7.36.11","oldver":null},{"name":"x11-wm/hypr","newver":"1.1.4","oldver":null}]
$ ./bumpbot --file overlay.toml --name "app-editors/cursor" --newver "7.36.11" --oldver ""
```

- [ ] bumpbot 生成 json 触发 gentoo-deps 生成 go/rust/npm tarball
- [ ] gentoo-deps actions 生成 tarball 之后，将软件包信息传递给 bumpbot，由 bumpbot 生成 PR
