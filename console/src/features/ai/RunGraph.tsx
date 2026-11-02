import { CheckCircle2, Circle, Hand, LoaderCircle, MinusCircle, Repeat, XCircle } from "lucide-react";
import { RunStep, StepStatus } from "./types";
import { formatDuration } from "./useRun";

export type GraphNode = Pick<RunStep, "id" | "title" | "status" | "durationMs"> & { agent?: string; detail?: string; after?: string[]; kind?: string };

function NodeIcon({ status, kind }: { status: StepStatus; kind?: string }) {
  if (kind === "human" && status !== "done") return <Hand size={16} />;
  switch (status) {
    case "running": return <LoaderCircle className="spin" size={16} />;
    case "done": return <CheckCircle2 size={16} />;
    case "failed": return <XCircle size={16} />;
    case "skipped": return <MinusCircle size={16} />;
    default: return <Circle size={16} />;
  }
}

/** A node whose `after` names a step that comes later in the list is a loop back (verify -> tailor). */
function loopsFrom(nodes: GraphNode[], index: number): string[] {
  const position = new Map(nodes.map((node, i) => [node.id, i]));
  return (nodes[index].after ?? []).filter((id) => (position.get(id) ?? -1) > index).map((id) => nodes.find((node) => node.id === id)?.title ?? id);
}

export function RunGraph({ nodes, waitingOn }: { nodes: GraphNode[]; waitingOn?: string }) {
  return <ol className="ai-graph" aria-label="Agent graph">
    {nodes.map((node, index) => {
      const loops = loopsFrom(nodes, index);
      const waiting = waitingOn === node.id;
      return <li key={node.id} className={`ai-graph-node ${node.status}${waiting ? " waiting" : ""}`}>
        <div className="ai-graph-card" title={node.detail}>
          <span className="ai-graph-icon"><NodeIcon status={node.status} kind={node.kind} /></span>
          <strong>{node.title}</strong>
          {node.agent && <span className="ai-graph-agent">{node.agent}</span>}
          {node.durationMs > 0 && <time>{formatDuration(node.durationMs)}</time>}
          {loops.length > 0 && <span className="ai-graph-loop"><Repeat size={12} /> can send work back to {loops.join(", ")}</span>}
        </div>
        {index < nodes.length - 1 && <span className="ai-graph-edge" aria-hidden="true" />}
      </li>;
    })}
  </ol>;
}
