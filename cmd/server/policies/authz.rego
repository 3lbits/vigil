package authz

import rego.v1

default allow := false

# Admins can do anything.
allow if {
	input.user.role == "admin"
	not risks_owner_decision_action
}

risks_owner_decision_action if {
	input.resource == "risks"
	input.action == "accept"
}

risks_owner_decision_action if {
	input.resource == "risks"
	input.action == "decline"
}

# Admins can accept/decline risk assessments only when owner.
allow if {
	input.user.role == "admin"
	input.resource == "risks"
	input.action in {"accept", "decline"}
	input.is_owner == true
}

# Editors can read, write, and delete data resources.
allow if {
	input.user.role == "editor"
	input.action in {"read", "write", "delete"}
	input.resource in {"frameworks", "requirements", "measures", "assets", "dashboard", "activities", "risk", "about", "avvik", "me"}
}

# Editors can update any resource (no participant check).
allow if {
	input.user.role == "editor"
	input.action == "update_own"
	input.resource in {"frameworks", "requirements", "measures", "assets", "dashboard", "activities", "risk", "about", "avvik", "me"}
}

# Contributors can create (write) and read.
allow if {
	input.user.role == "contributor"
	input.action in {"read", "write"}
	input.resource in {"frameworks", "requirements", "measures", "dashboard", "activities", "risk", "about", "avvik", "me"}
}

# Contributors can update only resources they participate in.
allow if {
	input.user.role == "contributor"
	input.action == "update_own"
	input.is_participant == true
	input.resource in {"frameworks", "requirements", "measures", "assets", "activities", "risk", "avvik"}
}

# Risk owners (editors/admins) can accept or decline pending assessments.
allow if {
	input.resource == "risks"
	input.action in {"accept", "decline"}
	input.user.role in {"editor", "admin"}
	input.is_owner == true
}

# Viewers can only read data resources.
allow if {
	input.user.role == "viewer"
	input.action == "read"
	input.resource in {"frameworks", "requirements", "measures", "assets", "dashboard", "activities", "risk", "about", "avvik", "me"}
}

# Scoped risk visibility: viewer can only read when connected or public.
allow if {
	input.resource == "risk"
	input.action == "read_scoped"
	input.user.role == "viewer"
	input.is_public == true
}

allow if {
	input.resource == "risk"
	input.action == "read_scoped"
	input.user.role == "viewer"
	input.is_participant == true
}

allow if {
	input.resource == "risk"
	input.action == "read_scoped"
	input.user.role == "viewer"
	input.is_owner == true
}

allow if {
	input.resource == "risk"
	input.action == "read_scoped"
	input.user.role == "viewer"
	input.is_creator == true
}

# Other non-viewer roles keep full risk read access.
allow if {
	input.resource == "risk"
	input.action == "read_scoped"
	input.user.role in {"contributor", "editor", "admin"}
}

# Risk owner or creator can toggle assessment visibility.
allow if {
	input.resource == "risk"
	input.action == "toggle_public"
	input.user.role in {"contributor", "editor"}
	input.is_owner == true
}

allow if {
	input.resource == "risk"
	input.action == "toggle_public"
	input.user.role in {"contributor", "editor"}
	input.is_creator == true
}

# Avvik reporter/creator can add notes and attachments.
allow if {
	input.resource == "avvik"
	input.action == "submit_own"
	input.user.role in {"contributor", "editor"}
	input.is_participant == true
}
