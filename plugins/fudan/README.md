# 复旦大学

Coursive 的来源插件：用复旦 UIS 登录，通过 WebVPN 读取 iCourse 上的课程、录播视频、课件和幻灯片截图，以及 Canvas（elearning）上的课程、作业、公告和文件。

## 登录

在 [UIS 账号安全](https://id.fudan.edu.cn/uc/#/pc/userCenter/accountSecurity) 中绑定「动态口令」，保存绑定时显示的 Base32 密钥。在 Coursive 的「来源 → 复旦大学」中填写学号、密码和这串密钥。认证器上每 30 秒变化的六位数字由插件在登录时生成。

- 密码和动态口令绑定密钥保存在 Coursive 守护进程的加密保管库中，插件本身不写任何文件。
- iCourse 只能在校内访问，插件把请求和下载地址都改写为 WebVPN 地址；WebVPN 的会话 Cookie 只交给守护进程，不会返回给应用。
- WebVPN 会话失效时，插件先用 UIS 会话静默登录，再用保存的密码和绑定密钥重新登录。旧会话遇到二次验证时需要在「来源」中补填密钥。续期失败时，「来源」会提示重新登录。

## 访问的网站

`id.fudan.edu.cn`、`webvpn.fudan.edu.cn`、`icourse.fudan.edu.cn`（经 WebVPN）、`elearning.fudan.edu.cn`。

## 构建

```sh
CGO_ENABLED=0 go build -trimpath -o plugins/fudan/bin/provider-linux-x64 ./plugins/fudan
```
