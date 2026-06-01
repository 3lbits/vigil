-- +goose Up
CREATE TABLE risk_assessment_participants (
    assessment_id UUID NOT NULL REFERENCES risk_assessments(id) ON DELETE CASCADE,
    user_id       UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    PRIMARY KEY (assessment_id, user_id)
);

-- +goose Down
DROP TABLE IF EXISTS risk_assessment_participants;
