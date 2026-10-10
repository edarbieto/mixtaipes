CREATE TABLE mixtapes (
    id          uuid PRIMARY KEY,
    client_hash bytea NOT NULL,
    locale      text NOT NULL CHECK (locale IN ('en', 'es')),
    market      text NOT NULL CHECK (length(market) = 2),
    created_at  timestamptz NOT NULL DEFAULT now(),
    updated_at  timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX mixtapes_client_hash_updated_at_idx ON mixtapes (client_hash, updated_at DESC);

CREATE TABLE turns (
    id                 uuid PRIMARY KEY,
    mixtape_id         uuid NOT NULL REFERENCES mixtapes (id) ON DELETE CASCADE,
    seq                smallint NOT NULL CHECK (seq BETWEEN 1 AND 20),
    user_prompt        text NOT NULL CHECK (char_length(user_prompt) <= 500),
    status             text NOT NULL CHECK (status IN ('ok', 'refused', 'failed')),
    refusal_reason     text CHECK (refusal_reason IN ('off_topic', 'unsafe', 'unclear')),
    error_code         text,
    prompt_version     text NOT NULL,
    llm_model          text NOT NULL,
    llm_output         jsonb,
    llm_calls          smallint NOT NULL DEFAULT 0,
    input_tokens       integer NOT NULL DEFAULT 0,
    output_tokens      integer NOT NULL DEFAULT 0,
    llm_latency_ms     integer NOT NULL DEFAULT 0,
    catalog_latency_ms integer NOT NULL DEFAULT 0,
    created_at         timestamptz NOT NULL DEFAULT now(),
    updated_at         timestamptz NOT NULL DEFAULT now(),
    UNIQUE (mixtape_id, seq)
);

CREATE INDEX turns_created_at_idx ON turns (created_at);

CREATE TABLE turn_tracks (
    turn_id          uuid NOT NULL REFERENCES turns (id) ON DELETE CASCADE,
    position         smallint NOT NULL CHECK (position BETWEEN 1 AND 30),
    llm_artist       text NOT NULL,
    llm_title        text NOT NULL,
    why              text NOT NULL,
    match_status     text NOT NULL CHECK (match_status IN ('matched', 'not_found')),
    backfilled       boolean NOT NULL DEFAULT false,
    spotify_track_id text,
    spotify_name     text,
    spotify_artists  text[],
    album_image_url  text,
    created_at       timestamptz NOT NULL DEFAULT now(),
    updated_at       timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (turn_id, position)
);

CREATE TABLE shares (
    slug       text PRIMARY KEY CHECK (length(slug) = 10),
    turn_id    uuid NOT NULL UNIQUE REFERENCES turns (id) ON DELETE CASCADE,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE events (
    id         bigserial PRIMARY KEY,
    type       text NOT NULL CHECK (type IN ('copy', 'preview_play', 'share_created', 'share_view')),
    mixtape_id uuid REFERENCES mixtapes (id) ON DELETE SET NULL,
    turn_id    uuid REFERENCES turns (id) ON DELETE SET NULL,
    share_slug text REFERENCES shares (slug) ON DELETE SET NULL,
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX events_type_created_at_idx ON events (type, created_at);
