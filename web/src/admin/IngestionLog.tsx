import { listIngestionLog } from "./adminApi";
import { useLoad, whenLong } from "./useLoad";

export function IngestionLog({ embeddingsOn }: { embeddingsOn: boolean }) {
  const log = useLoad(() => (embeddingsOn ? listIngestionLog() : Promise.resolve([])), [embeddingsOn]);
  return (
    <>
      <header className="page-head">
        <h1 id="view-title">Ingestion log</h1>
        <p>What was added, when, by whom, and to which teams. Later extensions do not change this record.</p>
      </header>
      {log.error && <p className="problem-line" role="alert">{log.error}</p>}
      {log.loading && !log.data && <p className="quiet" role="status">Loading the log.</p>}
      {log.data && log.data.length === 0 && <p className="quiet">Nothing has been ingested yet.</p>}
      {log.data && log.data.length > 0 && (
        <table className="table log">
          <thead>
            <tr>
              <th scope="col">When</th>
              <th scope="col">Document</th>
              <th scope="col">Added by</th>
              <th scope="col">Teams at upload</th>
              <th scope="col" className="num">Passages</th>
            </tr>
          </thead>
          <tbody>
            {log.data.map((e) => (
              <tr key={e.document_id + e.at}>
                <td data-label="When" className="nowrap">{whenLong(e.at)}</td>
                <th scope="row" data-label="Document">
                  <span className="doc-title">{e.title}</span>
                  <span className="doc-file">{e.source_filename}</span>
                </th>
                <td data-label="Added by">{e.by ?? "unknown"}</td>
                <td data-label="Teams at upload">
                  <span className="tags">{e.teams.map((t) => <span key={t} className="tag">{t}</span>)}</span>
                </td>
                <td className="num" data-label="Passages">{e.passages}</td>
              </tr>
            ))}
          </tbody>
        </table>
      )}
    </>
  );
}
