module github.com/rockbenben/launchbay

// 故意不写 toolchain 指令，别"顺手补上"。
//
// `go 1.27` 已经覆盖整条 1.27 线：CI 的 actions/setup-go 用 go-version-file 读
// 这份文件，取到的版本 spec 是 "1.27"，它在 1.27.x 里挑最新补丁（现在 1.27.2）；
// Dockerfile 的 golang:1.27-alpine 同样跟着走。三个环境（本机 / CI / 镜像）
// 已经是同一口径，不需要把补丁号抄进仓库——抄进去才是新的漂移源。
//
// 反过来，写上 toolchain go1.27.2 会让本机还装着 1.27.0 的人被 GOTOOLCHAIN
// 拖着自动下载一整套工具链：为一个不存在的问题，换一次几十 MB 的联网下载，
// 而这台机器上的东西按设计就该能在没外网的局域网里编出来。
go 1.27
