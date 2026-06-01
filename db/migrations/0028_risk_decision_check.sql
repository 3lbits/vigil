-- +goose Up
ALTER TABLE risks
ADD CONSTRAINT risks_risk_decision_check
CHECK (risk_decision IN ('', 'accept', 'treat', 'document'));

-- +goose Down
ALTER TABLE risks DROP CONSTRAINT IF EXISTS risks_risk_decision_check;
