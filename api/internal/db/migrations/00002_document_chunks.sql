-- +goose Up
-- +goose StatementBegin

-- The embedding size is 1024, confirmed against the live embeddings endpoint
-- (voyage-4-lite returns 1024 by default and honours 256, 512, 1024 and 2048).
-- Changing it later means re-embedding every chunk, so the API pins
-- output_dimension to 1024 on every call and refuses any other size.
--
-- No ANN index on purpose. At this scale an exact scan is fast, and an index
-- built for a filtered search can return fewer rows than asked for. See the
-- Limits section of the README.
--
-- char_start and char_end are UTF-16 code unit offsets into documents.content,
-- not byte offsets, so the browser can slice the stored text directly.
CREATE TABLE document_chunks (
    id          bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    document_id bigint NOT NULL REFERENCES documents (id) ON DELETE CASCADE,
    chunk_index integer NOT NULL CHECK (chunk_index >= 0),
    content     text NOT NULL CHECK (length(btrim(content)) > 0),
    embedding   vector(1024) NOT NULL,
    char_start  integer NOT NULL CHECK (char_start >= 0),
    char_end    integer NOT NULL,
    CONSTRAINT chunk_span_is_ordered CHECK (char_end > char_start),
    CONSTRAINT chunk_position_is_unique UNIQUE (document_id, chunk_index)
);
-- +goose StatementEnd

-- +goose Down
DROP TABLE document_chunks;
