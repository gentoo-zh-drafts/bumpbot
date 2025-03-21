gentoo-zh overlay bumpbot ，也可用于其他 overlay（大概）

GITHUB_TOKEN="ghp_xxxx" ./bumpbot -f overlay.toml

版本信息来自 `nvcmp --file overlay.toml --json --newer` 

功能:
 - [] 读取 nvcmp 传递过来的 json，由 bumpbot 创建 bump issues
 - [] bumpbot 生成触发 gentoo-deps 所用的 json，触发 gentoo-deps actions 生成 go/rust/npm tarball
 - [] gentoo-deps actions 生成 tarball 之后，将软件包信息传递给 bumpbot，由 bumpbot 生成 PR
