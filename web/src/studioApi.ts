import { api, id } from "./api";
export type StudioTab =
  | "collaboration"
  | "canvas"
  | "planning"
  | "scenes"
  | "characters"
  | "references"
  | "wiki"
  | "sandbox"
  | "branches"
  | "workflow"
  | "production"
  | "models"
  | "publishing";
export type StudioItem = {
  id: string;
  name?: string;
  title?: string;
  status?: string;
  createdAt?: string;
  [key: string]: unknown;
};
export const studioApi = {
  get: <T>(projectId: string, path: string) =>
    api<T>(`/projects/${projectId}/${path}`),
  post: <T>(projectId: string, path: string, body: unknown) =>
    api<T>(`/projects/${projectId}/${path}`, "POST", body),
  put: <T>(projectId: string, path: string, body: unknown) =>
    api<T>(`/projects/${projectId}/${path}`, "PUT", body),
  remove: (projectId: string, path: string) =>
    api(`/projects/${projectId}/${path}`, "DELETE"),
  intent: id,
};
export function strings(value: string) {
  return value
    .split(/[，,\n]/)
    .map((s) => s.trim())
    .filter(Boolean);
}
export function title(item: StudioItem) {
  return item.title || item.name || item.id;
}
