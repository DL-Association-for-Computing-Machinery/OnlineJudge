---
name: ECUT OJ
description: 简洁的桌面刷题工作台
colors:
  text: "#152033"
  muted: "#667085"
  line: "#e6e9ee"
  surface: "#ffffff"
  soft: "#fafbfc"
  accent: "#c4510a"
  accent-hover: "#a94305"
  accent-soft: "#fff6ed"
typography:
  body:
    fontFamily: '-apple-system, BlinkMacSystemFont, "Segoe UI", "PingFang SC", "Microsoft YaHei", sans-serif'
    fontSize: "15px"
    lineHeight: 1.9
  heading:
    fontFamily: '-apple-system, BlinkMacSystemFont, "Segoe UI", "PingFang SC", "Microsoft YaHei", sans-serif'
    fontSize: "36px"
    fontWeight: 650
    lineHeight: 1.4
rounded:
  control: "6px"
spacing:
  control-gap: "8px"
  group-gap: "16px"
components:
  button-primary:
    backgroundColor: "{colors.accent}"
    textColor: "{colors.surface}"
    rounded: "{rounded.control}"
    padding: "10px 18px"
  button-primary-hover:
    backgroundColor: "{colors.accent-hover}"
---
# ECUT OJ Web 设计约定

当前方向由用户确认的首页、登录页和做题页稿件决定：简洁桌面刷题工具，以内容和操作为中心。实现状态和边界见 [Web 界面](docs/frontend-design.md)，不要用视觉稿中的成功状态暗示未实现的能力。

## 基础 token

| 用途 | 值 |
| --- | --- |
| 正文 `--text` | `#152033` |
| 辅助文字 `--muted` | `#667085` |
| 分隔线 `--line` | `#e6e9ee` |
| 页面/控件 `--surface` | `#ffffff` |
| 辅助底色 `--soft` | `#fafbfc` |
| 操作 `--accent` | `#c4510a` |
| 操作悬停 `--accent-hover` | `#a94305` |
| 行悬停 `--accent-soft` | `#fff6ed` |
| 过渡曲线 `--ease` | `cubic-bezier(.2, .8, .2, 1)` |

字体使用本机系统中文/西文无衬线栈；代码用系统等宽字体，不请求外部字体。首页标题 36px，登录标题 32px，题目标题 30px；正文 14–17px，题面正文 15px/1.9。控件最小高度 44px，圆角 6px，焦点为橙色 2px 轮廓加 4px 间距。不要把低对比的亮橙直接用作小号正文或白字按钮背景。

## 布局与行为

- 顶部横向导航，高度约 71px，品牌在左、账号入口在右；当前栏目有细橙色下划线。做题预览归题库栏目。
- 首页内容最大宽 960px，只保留标题、搜索/折叠筛选、开放表格和分页；每页六题，无侧栏、统计卡片和宣传区。
- 登录内容最大宽 400px，单列窄表单、完整宽度主按钮；恢复操作默认折叠，状态提示在操作附近。
- 做题页初始题面/编辑区为 46/54，分隔条可拖动或用方向键/Home/End 调整至 30–65；题面和代码分别滚动。
- 线条承担分组，留白承担层级；阴影只在筛选浮层等需要表达覆盖关系的地方使用。图标使用同一套 19px 线形 SVG。
- 按钮/导航/行背景过渡 150ms，折叠内容淡入 160ms；无全页入场编排。减少动画偏好下关闭动画、过渡和按压位移。
- 所有操作必须覆盖键盘焦点、禁用、加载和失败状态；示例代码临时保留及提交未开放的文案应始终真实。

CSS 的权威来源是 [全局样式](web/src/styles.css)、[账号样式](web/src/account/account.css)及[做题样式](web/src/preview/workspace.css)。后续修改沿用这些 token，不重新拼装另一套视觉语言。
