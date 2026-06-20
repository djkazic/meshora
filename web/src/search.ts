import type { MeshMap } from "./map";
import { esc, roleAbbrev } from "./util";

export class NodeSearch {
  private input: HTMLInputElement;
  private results: HTMLElement;

  constructor(
    private mesh: MeshMap,
    private onSelect: (pubkey: string) => void,
  ) {
    this.input = document.querySelector("#nodesearch .ns-input")!;
    this.results = document.querySelector("#nodesearch .ns-results")!;

    this.input.addEventListener("input", () => this.render());
    this.input.addEventListener("focus", () => this.render());
    this.input.addEventListener("keydown", (e) => {
      if (e.key === "Enter") {
        const first = this.results.querySelector<HTMLElement>("[data-pk]");
        if (first) this.select(first.dataset.pk!);
      } else if (e.key === "Escape") {
        this.clear();
        this.input.blur();
      }
    });
    this.input.addEventListener("blur", () => setTimeout(() => this.hide(), 150));
  }

  private render(): void {
    const q = this.input.value.trim();
    if (!q) {
      this.hide();
      return;
    }
    const matches = this.mesh.search(q, 8);
    if (!matches.length) {
      this.results.innerHTML = '<div class="ns-empty">no nodes</div>';
      this.results.classList.add("show");
      return;
    }
    this.results.innerHTML = matches
      .map((n) => {
        const ab = roleAbbrev(n.role);
        const tag = ab ? ` <i class="ns-role">(${ab})</i>` : "";
        const name = esc(n.name || n.pubkey.slice(0, 10));
        return `<div class="ns-item" data-pk="${esc(n.pubkey)}">${name}${tag}</div>`;
      })
      .join("");
    this.results.classList.add("show");
    this.results.querySelectorAll<HTMLElement>("[data-pk]").forEach((el) =>
      el.addEventListener("mousedown", (e) => {
        e.preventDefault();
        this.select(el.dataset.pk!);
      }),
    );
  }

  private select(pubkey: string): void {
    this.mesh.focus(pubkey);
    this.onSelect(pubkey);
    this.clear();
  }

  private clear(): void {
    this.input.value = "";
    this.hide();
  }

  private hide(): void {
    this.results.classList.remove("show");
  }
}
