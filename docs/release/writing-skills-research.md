# 小说写作 Skill 来源与选择

核验日期：2026-11-13。下列条目已加入页面“精选指南”，按固定提交导入，不随远端默认分支自动改变。许可证根据上游该提交的LICENSE核对；推荐不等于已完成文学质量A/B评估。

| Skill | 主要用途 | 许可证 | 兼容性 |
|---|---|---|---|
| [Story Deslop · 中文网文表达修订](https://github.com/ohbiaji/write-skill/tree/8aaa701a783a3065144b2df2faac9fa70e9c8f7b/skills/story-deslop) | 针对解释腔、模板化抒情、重复动作和过度工整的中文网文审读与最小修改。 | MIT | 中文长技能，建议只用于润色和Copilot。附上游CLI、子代理和脚本要求，砚台会提示并忽略这些执行指令，只加载文档；禁用词和标点规则需按作者文风取舍。 |
| [Writing Principles · 读者体验与去套路](https://github.com/haowjy/creative-writing-skills/tree/0fb3c580a4e20435ddb0c0a744629b42a63c7307/skills/writing-principles) | 信任读者、叙事密度与模型常见失败模式；适合起草和审稿。 | Apache-2.0 | 英文指南。附resources；建议先阅读正文及失败模式，再按作品调整。 |
| [Creative Writing Craft · 小说技法](https://github.com/haowjy/creative-writing-skills/tree/0fb3c580a4e20435ddb0c0a744629b42a63c7307/skills/creative-writing-craft) | 场景、叙述距离、自由间接引语、节奏与文风样例分析的按需资料入口。 | Apache-2.0 | 主文件很短，核心内容在resources，推荐通过Copilot按需读取；不需要照搬上游项目目录。 |
| [Story Review · 分层审稿](https://github.com/haowjy/creative-writing-skills/tree/0fb3c580a4e20435ddb0c0a744629b42a63c7307/skills/story-review) | 区分结构、发展性编辑、逐句编辑与校对，先诊断再改写。 | Apache-2.0 | 按审稿目标选用参考材料；上游其他skill并非砚台已自动安装的工具。 |
| [Scene Craft · 场景与潜台词](https://github.com/danjdewhurst/story-skills/tree/74dd2f92dd757d4b262085bddccb09ed5a4281d1/skills/scene-craft) | 场景及回落、尝试与代价、对白潜台词、深视角和说明信息安排。 | MIT | 英文资料，原项目含CLI和跨skill引用；砚台只加载文档与本包references，不运行story命令。 |
| [Humanizer · 通用表达审读](https://github.com/blader/humanizer/blob/b48c832f0f809c1b5658edbb838c2521b72f2413/SKILL.md) | 识别模板对比、空泛总结、整齐排比、过度格式化与夸大表达。 | MIT | 通用文本审读，不是小说专用规则。部分建议面向英文；不能机械删除小说必要的节奏、情感和修辞。 |

## 固定来源

- story-deslop：[https://github.com/ohbiaji/write-skill](https://github.com/ohbiaji/write-skill)；提交`8aaa701a783a3065144b2df2faac9fa70e9c8f7b`。
- writing-principles：[https://github.com/haowjy/creative-writing-skills](https://github.com/haowjy/creative-writing-skills)；提交`0fb3c580a4e20435ddb0c0a744629b42a63c7307`。
- creative-writing-craft：[https://github.com/haowjy/creative-writing-skills](https://github.com/haowjy/creative-writing-skills)；提交`0fb3c580a4e20435ddb0c0a744629b42a63c7307`。
- story-review：[https://github.com/haowjy/creative-writing-skills](https://github.com/haowjy/creative-writing-skills)；提交`0fb3c580a4e20435ddb0c0a744629b42a63c7307`。
- scene-craft：[https://github.com/danjdewhurst/story-skills](https://github.com/danjdewhurst/story-skills)；提交`74dd2f92dd757d4b262085bddccb09ed5a4281d1`。
- humanizer：[https://github.com/blader/humanizer](https://github.com/blader/humanizer)；提交`b48c832f0f809c1b5658edbb838c2521b72f2413`。

StoryDeslop最贴近中文表达修订；先预览其词句/标点约束，删除与自己语气不合的规则。Writing Principles强调读者信任、具体动作与避免过度解释，适合与作者文风样例结合。Creative Writing Craft与Story Review用于分层创作与审阅；Scene Craft适合场景转折、潜台词和视角。Humanizer偏英文通用编辑，不能把通用“人性化”检查单当成小说质量标准。

推荐配置是一个叙事指南配一个对白或作者文风指南，再把较长审稿Skill只分配给审阅任务。内置五个短中文模板由本项目独立编写，并非从第三方主文件复制。第三方正文和许可通过作者预览后导入到作品，不作为本项目原始代码授权。

另检查了[lenmo181/novel-writing-skill](https://github.com/lenmo181/novel-writing-skill)（Apache-2.0，提交`a36e156087b3e237088cc2b0e754cf4e5237fb06`）及[xianzl/novel-style-skills](https://github.com/xianzl/novel-style-skills)（MIT，提交`649308e305d1cda1ce4b1f9154115981e9240713`）。前者主文件和脚本/配套体系较大，后者依赖Python文风加载器；未把这些外部执行依赖误包装为应用已支持的一键指南。需要时可整理为独立SKILL.md与可读引用后导入。

## 实际在线导入

六项固定提交均用真实GitHub获取与应用导入器预览成功：writing-principles 4份引用、creative-writing-craft 11份、story-review 14份、scene-craft 9份、humanizer 6份、story-deslop 3份。引用数含保留的LICENSE/NOTICE；main SKILL.md单独保存。URL、提交、内容哈希、许可与兼容性warning摘要位于本地.local/skill-import-previews；本轮仅抓取公开指南，没有额外模型收费，也没有自动启用到作者作品。

模型文学效果、作者修改时间和长期采用率继续后置；本功能提供可控约束、来源和复查能力，不能保证零“AI味”。
