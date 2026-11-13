import type { AuthorMemoryInput } from "./AuthorMemories";

export type OpenAuthorMemoryRequest = {
  projectId?: string;
  initialDraft?: Partial<AuthorMemoryInput>;
};
export const AUTHOR_MEMORY_OPEN = "yantai-open-author-memories";
export function openAuthorMemories(request: OpenAuthorMemoryRequest = {}) {
  window.dispatchEvent(
    new CustomEvent<OpenAuthorMemoryRequest>(AUTHOR_MEMORY_OPEN, {
      detail: request,
    }),
  );
}
