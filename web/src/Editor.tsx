import { useEffect, useRef, useState } from "react";
import { EditorContent, useEditor } from "@tiptap/react";
import StarterKit from "@tiptap/starter-kit";
import {
  Bold,
  Italic,
  Heading2,
  List,
  Quote,
  Undo2,
  Redo2,
  Type,
  X,
  Sparkles,
  WandSparkles,
  Minimize2,
  Maximize2,
} from "lucide-react";
import type { Doc } from "./api";
import "./editorExperience.css";

type ReadingStyle = {
  fontSize: number;
  lineHeight: number;
  family: "serif" | "sans";
};
const defaultReadingStyle: ReadingStyle = {
  fontSize: 18,
  lineHeight: 2.1,
  family: "serif",
};
function readReadingStyle(): ReadingStyle {
  try {
    const value = JSON.parse(
      localStorage.getItem("yantai-reading-style") || "null",
    );
    return {
      fontSize: Math.max(14, Math.min(26, Number(value?.fontSize) || 18)),
      lineHeight: Math.max(
        1.6,
        Math.min(2.8, Number(value?.lineHeight) || 2.1),
      ),
      family: value?.family === "sans" ? "sans" : "serif",
    };
  } catch {
    return defaultReadingStyle;
  }
}
export default function Editor({
  initial,
  onChange,
  onSelection,
  onAskAI,
  resetKey = 0,
  readOnly = false,
}: {
  initial: Doc;
  onChange: (doc: Doc) => void;
  onSelection?: (text: string) => void;
  onAskAI?: (request: string) => void;
  resetKey?: number;
  readOnly?: boolean;
}) {
  const [readingStyle, setReadingStyle] = useState(readReadingStyle);
  const [styleOpen, setStyleOpen] = useState(false);
  const [selectionBubble, setSelectionBubble] = useState<{
    left: number;
    top: number;
    text: string;
  } | null>(null);
  const editorSurface = useRef<HTMLDivElement>(null);
  const styleButton = useRef<HTMLButtonElement>(null);
  useEffect(() => {
    try {
      localStorage.setItem(
        "yantai-reading-style",
        JSON.stringify(readingStyle),
      );
    } catch {
      /* Reading preferences never prevent writing. */
    }
  }, [readingStyle]);
  const editor = useEditor({
    immediatelyRender: false,
    extensions: [StarterKit],
    content: initial,
    editable: !readOnly,
    onUpdate: ({ editor }) => onChange(editor.getJSON() as Doc),
    onSelectionUpdate: ({ editor }) => {
      const { from, to } = editor.state.selection;
      if (from !== to)
        onSelection?.(editor.state.doc.textBetween(from, to, "\n"));
    },
    editorProps: {
      attributes: {
        class: "manuscript",
        role: "textbox",
        "aria-label": "章节正文",
        "data-testid": "chapter-editor",
      },
    },
  });
  useEffect(() => {
    editor?.setEditable(!readOnly, false);
  }, [editor, readOnly]);
  useEffect(() => {
    if (editor && !editor.isDestroyed && resetKey > 0)
      editor.commands.setContent(initial, { emitUpdate: false });
  }, [editor, resetKey]);
  useEffect(() => {
    if (!editor) return;
    const refresh = () => {
      const { from, to } = editor.state.selection;
      if (from === to || !editor.isFocused || readOnly || !onAskAI) {
        setSelectionBubble(null);
        return;
      }
      const text = editor.state.doc.textBetween(from, to, "\n");
      if (!text.trim()) {
        setSelectionBubble(null);
        return;
      }
      const a = editor.view.coordsAtPos(from),
        b = editor.view.coordsAtPos(to);
      const host = editorSurface.current?.getBoundingClientRect();
      if (!host || b.bottom < host.top || a.top > host.bottom) {
        setSelectionBubble(null);
        return;
      }
      const width = Math.min(320, Math.max(220, host.width - 16));
      setSelectionBubble({
        left: Math.max(
          8,
          Math.min(
            window.innerWidth - width - 8,
            (a.left + b.left) / 2 - width / 2,
          ),
        ),
        top: Math.max(8, Math.min(window.innerHeight - 52, a.top - 46)),
        text,
      });
    };
    editor.on("selectionUpdate", refresh);
    editor.on("focus", refresh);
    const onMouseUp = () => refresh();
    const onBlur = () => setSelectionBubble(null);
    editorSurface.current?.addEventListener("mouseup", onMouseUp);
    editor.on("blur", onBlur);
    window.addEventListener("scroll", onBlur, true);
    window.addEventListener("resize", onBlur);
    return () => {
      editor.off("selectionUpdate", refresh);
      editor.off("focus", refresh);
      editor.off("blur", onBlur);
      editorSurface.current?.removeEventListener("mouseup", onMouseUp);
      window.removeEventListener("scroll", onBlur, true);
      window.removeEventListener("resize", onBlur);
    };
  }, [editor, onAskAI, readOnly]);
  if (!editor) return null;
  const buttons = [
    {
      Icon: Bold,
      label: "加粗",
      active: editor.isActive("bold"),
      run: () => editor.chain().focus().toggleBold().run(),
    },
    {
      Icon: Italic,
      label: "斜体",
      active: editor.isActive("italic"),
      run: () => editor.chain().focus().toggleItalic().run(),
    },
    {
      Icon: Heading2,
      label: "小标题",
      active: editor.isActive("heading", { level: 2 }),
      run: () => editor.chain().focus().toggleHeading({ level: 2 }).run(),
    },
    {
      Icon: List,
      label: "列表",
      active: editor.isActive("bulletList"),
      run: () => editor.chain().focus().toggleBulletList().run(),
    },
    {
      Icon: Quote,
      label: "引用",
      active: editor.isActive("blockquote"),
      run: () => editor.chain().focus().toggleBlockquote().run(),
    },
    {
      Icon: Undo2,
      label: "撤销",
      active: false,
      run: () => editor.chain().focus().undo().run(),
    },
    {
      Icon: Redo2,
      label: "重做",
      active: false,
      run: () => editor.chain().focus().redo().run(),
    },
  ];
  return (
    <div
      ref={editorSurface}
      className="editor-experience"
      style={
        {
          "--manuscript-font-size": `${readingStyle.fontSize}px`,
          "--manuscript-line-height": readingStyle.lineHeight,
          "--manuscript-font-family":
            readingStyle.family === "sans"
              ? '"PingFang SC", "Microsoft YaHei", sans-serif'
              : '"Songti SC", "STSong", "SimSun", Georgia, serif',
        } as React.CSSProperties
      }
    >
      <div className="editor-toolbar" role="toolbar" aria-label="正文格式">
        {buttons.map(({ Icon, label, active, run }) => (
          <button
            key={label}
            className={"icon-button " + (active ? "active" : "")}
            onClick={run}
            title={label}
            aria-label={label}
            disabled={readOnly}
          >
            <Icon size={16} />
          </button>
        ))}
        <span className="toolbar-note">正文</span>
        <button
          ref={styleButton}
          className={"editor-reading-toggle " + (styleOpen ? "active" : "")}
          aria-label="正文排版"
          aria-expanded={styleOpen}
          onClick={() => setStyleOpen(!styleOpen)}
        >
          <Type size={15} />
          <span>排版</span>
        </button>
      </div>
      {styleOpen && (
        <section className="editor-reading-settings" aria-label="正文显示设置">
          <header>
            <span>正文显示</span>
            <small>只影响阅读，不改变稿件</small>
            <button
              aria-label="关闭正文排版"
              onClick={() => {
                setStyleOpen(false);
                styleButton.current?.focus();
              }}
            >
              <X size={14} />
            </button>
          </header>
          <label>
            字号 <output>{readingStyle.fontSize}px</output>
            <input
              aria-label="正文字号"
              type="range"
              min="14"
              max="26"
              step="1"
              value={readingStyle.fontSize}
              onChange={(e) =>
                setReadingStyle({
                  ...readingStyle,
                  fontSize: Number(e.target.value),
                })
              }
            />
          </label>
          <label>
            行距 <output>{readingStyle.lineHeight.toFixed(1)}</output>
            <input
              aria-label="正文行距"
              type="range"
              min="1.6"
              max="2.8"
              step="0.1"
              value={readingStyle.lineHeight}
              onChange={(e) =>
                setReadingStyle({
                  ...readingStyle,
                  lineHeight: Number(e.target.value),
                })
              }
            />
          </label>
          <label>
            字体
            <select
              aria-label="正文字体"
              value={readingStyle.family}
              onChange={(e) =>
                setReadingStyle({
                  ...readingStyle,
                  family: e.target.value as ReadingStyle["family"],
                })
              }
            >
              <option value="serif">宋体 · 叙事阅读</option>
              <option value="sans">黑体 · 清晰阅读</option>
            </select>
          </label>
          <button
            className="text-button"
            onClick={() => setReadingStyle(defaultReadingStyle)}
          >
            恢复默认排版
          </button>
        </section>
      )}
      <EditorContent editor={editor} />
      {selectionBubble && onAskAI && (
        <div
          className="editor-selection-actions"
          role="toolbar"
          aria-label="选段创作操作"
          style={{ left: selectionBubble.left, top: selectionBubble.top }}
          onMouseDown={(event) => event.preventDefault()}
        >
          {[
            {
              label: "讨论",
              Icon: Sparkles,
              request:
                "请先分析当前选段的作用、节奏和人物动机，和我讨论改进方向，暂不生成候选。",
            },
            {
              label: "扩写",
              Icon: Maximize2,
              request:
                "请扩写当前选段，增加有作用的动作和细节，保持人物知识、事实与情节顺序，生成独立候选供我比较。",
            },
            {
              label: "润色",
              Icon: WandSparkles,
              request:
                "请按本书文风和启用的写作指南润色当前选段，保留事实、人物声音和潜台词，生成独立候选供我比较。",
            },
            {
              label: "压缩",
              Icon: Minimize2,
              request:
                "请压缩当前选段，删去重复解释与无作用的修饰，保留因果、信息和人物声音，生成独立候选供我比较。",
            },
          ].map(({ label, Icon, request }) => (
            <button
              key={label}
              aria-label={`${label}当前选段`}
              onClick={() => {
                onSelection?.(selectionBubble.text);
                onAskAI(request);
                setSelectionBubble(null);
              }}
            >
              <Icon size={13} />
              {label}
            </button>
          ))}
        </div>
      )}
    </div>
  );
}
