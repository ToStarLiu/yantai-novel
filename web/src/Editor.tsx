import { useEffect } from "react";
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
} from "lucide-react";
import type { Doc } from "./api";
export default function Editor({
  initial,
  onChange,
  onSelection,
  resetKey = 0,
  readOnly = false,
}: {
  initial: Doc;
  onChange: (doc: Doc) => void;
  onSelection?: (text: string) => void;
  resetKey?: number;
  readOnly?: boolean;
}) {
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
    <>
      <div className="editor-toolbar">
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
        <span className="toolbar-note">让故事慢慢发生</span>
      </div>
      <EditorContent editor={editor} />
    </>
  );
}
