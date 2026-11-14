import { useEffect, useId, useRef, useState, type ComponentType } from "react";
import { MoreHorizontal, X } from "lucide-react";
import "./workspaceActions.css";

export type WorkspaceAction = {
  id: string;
  label: string;
  group: string;
  Icon: ComponentType<{ size?: number }>;
  disabled?: boolean;
  onSelect: () => unknown | Promise<unknown>;
};

export default function WorkspaceActions({
  actions,
  onError,
}: {
  actions: WorkspaceAction[];
  onError: (error: unknown) => void;
}) {
  const [open, setOpen] = useState(false);
  const root = useRef<HTMLDivElement>(null);
  const trigger = useRef<HTMLButtonElement>(null);
  const panel = useRef<HTMLElement>(null);
  const panelId = useId();
  const close = (restoreFocus = false) => {
    setOpen(false);
    if (restoreFocus) trigger.current?.focus();
  };
  useEffect(() => {
    if (!open) return;
    const frame = requestAnimationFrame(() => {
      panel.current
        ?.querySelector<HTMLButtonElement>("button[data-action]:not(:disabled)")
        ?.focus();
    });
    const outside = (event: PointerEvent | FocusEvent) => {
      if (event.target instanceof Node && !root.current?.contains(event.target))
        setOpen(false);
    };
    const escape = (event: KeyboardEvent) => {
      if (event.key !== "Escape") return;
      event.preventDefault();
      event.stopPropagation();
      setOpen(false);
      trigger.current?.focus();
    };
    document.addEventListener("pointerdown", outside);
    document.addEventListener("focusin", outside);
    document.addEventListener("keydown", escape, true);
    return () => {
      cancelAnimationFrame(frame);
      document.removeEventListener("pointerdown", outside);
      document.removeEventListener("focusin", outside);
      document.removeEventListener("keydown", escape, true);
    };
  }, [open]);
  return (
    <div className="workspace-actions" ref={root}>
      <button
        ref={trigger}
        className="button workspace-actions-trigger"
        aria-label="更多操作"
        aria-haspopup="dialog"
        aria-expanded={open}
        aria-controls={panelId}
        onClick={() => setOpen((value) => !value)}
      >
        <MoreHorizontal size={18} />
        <span>更多</span>
      </button>
      {open && (
        <section
          id={panelId}
          ref={panel}
          className="workspace-actions-panel"
          role="dialog"
          aria-label="作品与设置"
          onKeyDown={(event) => {
            if (!["ArrowDown", "ArrowUp", "Home", "End"].includes(event.key))
              return;
            const buttons = Array.from(
              panel.current?.querySelectorAll<HTMLButtonElement>(
                "button[data-action]:not(:disabled)",
              ) || [],
            );
            if (!buttons.length) return;
            const current = buttons.indexOf(
              document.activeElement as HTMLButtonElement,
            );
            const index =
              event.key === "Home"
                ? 0
                : event.key === "End"
                  ? buttons.length - 1
                  : current < 0
                    ? event.key === "ArrowDown"
                      ? 0
                      : buttons.length - 1
                    : (current +
                        (event.key === "ArrowDown" ? 1 : -1) +
                        buttons.length) %
                      buttons.length;
            event.preventDefault();
            buttons[index]?.focus();
          }}
        >
          <header>
            <strong>作品与设置</strong>
            <button
              className="icon-button"
              aria-label="关闭更多操作"
              onClick={() => close(true)}
            >
              <X size={16} />
            </button>
          </header>
          {actions.map((action, index) => (
            <div key={action.id}>
              {(index === 0 || actions[index - 1].group !== action.group) && (
                <p className="workspace-action-group">{action.group}</p>
              )}
              <button
                data-action={action.id}
                className="workspace-action-item"
                disabled={action.disabled}
                onClick={() => {
                  close(true);
                  void Promise.resolve().then(action.onSelect).catch(onError);
                }}
              >
                <action.Icon size={16} />
                <span>{action.label}</span>
              </button>
            </div>
          ))}
        </section>
      )}
    </div>
  );
}
