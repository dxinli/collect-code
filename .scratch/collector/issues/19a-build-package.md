# 19a: 构建、systemd 打包与冒烟

**What to build:** 产物能装到设备上并按文档跑起来。

**Blocked by:** 01a

**Status:** ready-for-agent

- [ ] `Makefile` 产出 arm64/amd64 静态二进制（file/ldd 断言）
- [ ] systemd unit 示例与安装脚本可用
- [ ] 干净容器按脚本完成安装并启动成功
- [ ] 冒烟脚本退出码可判
