package httpapi.authz

import future.keywords

# paths
alerts_path := ["api", "v1", "alerts"]
alerts_definitions_path := ["api", "v1", "alerts", "definitions"]
alerts_definitions_uuid_path := ["api", "v1", "alerts", "definitions", "some-uuid-here"]
alerts_definitions_uuid_template_path := ["api", "v1", "alerts", "definitions", "some-uuid-here", "template"]
alerts_receivers_path := ["api", "v1", "alerts", "receivers"]
alerts_receivers_uuid_path := ["api", "v1", "alerts", "receivers", "some-uuid-here"]
alerts_receivers_uuid_template_path := ["api", "v1", "alerts", "receivers", "some-uuid-here", "template"]
# NOTE: current opa policies do not enforce a valid UUID structure, hence the "some-uuid-here"

all_get_paths := [alerts_path, alerts_definitions_path, alerts_definitions_uuid_path, alerts_definitions_uuid_template_path, alerts_receivers_path, alerts_receivers_uuid_path, alerts_receivers_uuid_template_path]
all_patch_paths := [alerts_definitions_uuid_path, alerts_definitions_uuid_template_path, alerts_receivers_path, alerts_receivers_uuid_path, alerts_receivers_uuid_template_path]

# roles
alerts_r := ["11111111-1111-1111-1111-111111111111_alrt-r"]
alerts_rw := ["11111111-1111-1111-1111-111111111111_alrt-rw"]
alerts_admin_r := ["alrt-r"]
alerts_admin_rw := ["alrt-rw"]
alert_admin_receivers_rw := ["alrt-rx-rw"]
unauthorized_role := ["unauthorized-role-example"]

test_get_valid_roles if {
    alerts_admin_r[0] in get_valid_roles(alerts_admin_r[0]) with input as {"path": alerts_path, "project": "11111111-1111-1111-1111-111111111111"}
    alerts_r[0] in get_valid_roles(alerts_admin_r[0]) with input as {"path": alerts_path, "project": "11111111-1111-1111-1111-111111111111"}
}

test_alerts_endpoint if {
    allow_alrt_r with input as {"roles":alerts_r, "method":"GET", "path":alerts_path, "project": "11111111-1111-1111-1111-111111111111"}
    allow_alrt_r with input as {"roles":alerts_admin_r, "method":"GET", "path":alerts_path, "project": "11111111-1111-1111-1111-111111111111"}
    allow_alrt_rw with input as {"roles":alerts_rw, "method":"GET", "path":alerts_path, "project": "11111111-1111-1111-1111-111111111111"}
    not allow_alert_rx_rw with input as {"roles":alert_admin_receivers_rw, "method":"GET", "path":alerts_path, "project": "11111111-1111-1111-1111-111111111111"}
}

test_alerts_definitions_get_endpoint if {
    # /edgenode/api/v1/alerts/definitions
    allow_alrt_r with input as {"roles":alerts_r, "method":"GET", "path":alerts_definitions_path, "project": "11111111-1111-1111-1111-111111111111"}
    allow_alrt_rw with input as {"roles":alerts_rw, "method":"GET", "path":alerts_definitions_path, "project": "11111111-1111-1111-1111-111111111111"}
    not allow_alert_rx_rw with input as {"roles":alert_admin_receivers_rw, "method":"GET", "path":alerts_definitions_path, "project": "11111111-1111-1111-1111-111111111111"}

    # /edgenode/api/v1/alerts/definitions/<uuid>
    allow_alrt_r with input as {"roles":alerts_r, "method":"GET", "path":alerts_definitions_uuid_path, "project": "11111111-1111-1111-1111-111111111111"}
    allow_alrt_rw with input as {"roles":alerts_rw, "method":"GET", "path":alerts_definitions_uuid_path, "project": "11111111-1111-1111-1111-111111111111"}
    not allow_alert_rx_rw with input as {"roles":alert_admin_receivers_rw, "method":"GET", "path":alerts_definitions_uuid_path, "project": "11111111-1111-1111-1111-111111111111"}

    # /edgenode/api/v1/alerts/definitions/<uuid>/template
    allow_alrt_r with input as {"roles":alerts_r, "method":"GET", "path":alerts_definitions_uuid_template_path, "project": "11111111-1111-1111-1111-111111111111"}
    allow_alrt_rw with input as {"roles":alerts_rw, "method":"GET", "path":alerts_definitions_uuid_template_path, "project": "11111111-1111-1111-1111-111111111111"}
    not allow_alert_rx_rw with input as {"roles":alert_admin_receivers_rw, "method":"GET", "path":alerts_definitions_uuid_template_path, "project": "11111111-1111-1111-1111-111111111111"}
}

test_alerts_definitions_patch_endpoint if {
    # /edgenode/api/v1/alerts/definitions
    not allow_alrt_r with input as {"roles":alerts_r, "method":"PATCH", "path":alerts_definitions_path, "project": "11111111-1111-1111-1111-111111111111"}
    allow_alrt_rw with input as {"roles":alerts_rw, "method":"PATCH", "path":alerts_definitions_path, "project": "11111111-1111-1111-1111-111111111111"}
    allow_alrt_rw with input as {"roles":alerts_admin_rw, "method":"PATCH", "path":alerts_definitions_path, "project": "11111111-1111-1111-1111-111111111111"}
    not allow_alert_rx_rw with input as {"roles":alert_admin_receivers_rw, "method":"PATCH", "path":alerts_definitions_path, "project": "11111111-1111-1111-1111-111111111111"}

    # /edgenode/api/v1/alerts/definitions/<uuid>
    not allow_alrt_r with input as {"roles":alerts_r, "method":"PATCH", "path":alerts_definitions_uuid_path, "project": "11111111-1111-1111-1111-111111111111"}
    allow_alrt_rw with input as {"roles":alerts_rw, "method":"PATCH", "path":alerts_definitions_uuid_path, "project": "11111111-1111-1111-1111-111111111111"}
    allow_alrt_rw with input as {"roles":alerts_admin_rw, "method":"PATCH", "path":alerts_definitions_uuid_path, "project": "11111111-1111-1111-1111-111111111111"}
    not allow_alert_rx_rw with input as {"roles":alert_admin_receivers_rw, "method":"PATCH", "path":alerts_definitions_uuid_path, "project": "11111111-1111-1111-1111-111111111111"}

    # /edgenode/api/v1/alerts/definitions/<uuid>/template
    not allow_alrt_r with input as {"roles":alerts_r, "method":"PATCH", "path":alerts_definitions_uuid_template_path, "project": "11111111-1111-1111-1111-111111111111"}
    allow_alrt_rw with input as {"roles":alerts_rw, "method":"PATCH", "path":alerts_definitions_uuid_template_path, "project": "11111111-1111-1111-1111-111111111111"}
    allow_alrt_rw with input as {"roles":alerts_admin_rw, "method":"PATCH", "path":alerts_definitions_uuid_template_path, "project": "11111111-1111-1111-1111-111111111111"}
    not allow_alert_rx_rw with input as {"roles":alert_admin_receivers_rw, "method":"PATCH", "path":alerts_definitions_uuid_template_path, "project": "11111111-1111-1111-1111-111111111111"}
}

test_alerts_receivers_get_endpoint if {
    # /edgenode/api/v1/alerts/receivers
    not allow_alrt_r with input as {"roles":alerts_admin_r, "method":"GET", "path":alerts_receivers_path, "project": "11111111-1111-1111-1111-111111111111"}
    not allow_alrt_rw with input as {"roles":alerts_admin_rw, "method":"GET", "path":alerts_receivers_path, "project": "11111111-1111-1111-1111-111111111111"}
    allow_alert_rx_rw with input as {"roles":alert_admin_receivers_rw, "method":"GET", "path":alerts_receivers_path, "project": "11111111-1111-1111-1111-111111111111"}

    # /edgenode/api/v1/alerts/receivers/<uuid>
    not allow_alrt_r with input as {"roles":alerts_admin_r, "method":"GET", "path":alerts_receivers_uuid_path, "project": "11111111-1111-1111-1111-111111111111"}
    not allow_alrt_rw with input as {"roles":alerts_admin_rw, "method":"GET", "path":alerts_receivers_uuid_path, "project": "11111111-1111-1111-1111-111111111111"}
    allow_alert_rx_rw with input as {"roles":alert_admin_receivers_rw, "method":"GET", "path":alerts_receivers_uuid_path, "project": "11111111-1111-1111-1111-111111111111"}

    # /edgenode/api/v1/alerts/receivers/<uuid>/template
    not allow_alrt_r with input as {"roles":alerts_admin_r, "method":"GET", "path":alerts_receivers_uuid_template_path, "project": "11111111-1111-1111-1111-111111111111"}
    not allow_alrt_rw with input as {"roles":alerts_admin_rw, "method":"GET", "path":alerts_receivers_uuid_template_path, "project": "11111111-1111-1111-1111-111111111111"}
    allow_alert_rx_rw with input as {"roles":alert_admin_receivers_rw, "method":"GET", "path":alerts_receivers_uuid_template_path, "project": "11111111-1111-1111-1111-111111111111"}
}

test_alerts_receivers_patch_endpoint if {
    # /edgenode/api/v1/alerts/receivers
    not allow_alrt_r with input as {"roles":alerts_admin_r, "method":"PATCH", "path":alerts_receivers_path, "project": "11111111-1111-1111-1111-111111111111"}
    not allow_alrt_rw with input as {"roles":alerts_admin_rw, "method":"PATCH", "path":alerts_receivers_path, "project": "11111111-1111-1111-1111-111111111111"}
    allow_alert_rx_rw with input as {"roles":alert_admin_receivers_rw, "method":"PATCH", "path":alerts_receivers_path, "project": "11111111-1111-1111-1111-111111111111"}

    # /edgenode/api/v1/alerts/receivers/<uuid>
    not allow_alrt_r with input as {"roles":alerts_admin_r, "method":"PATCH", "path":alerts_receivers_uuid_path, "project": "11111111-1111-1111-1111-111111111111"}
    not allow_alrt_rw with input as {"roles":alerts_admin_rw, "method":"PATCH", "path":alerts_receivers_uuid_path, "project": "11111111-1111-1111-1111-111111111111"}
    allow_alert_rx_rw with input as {"roles":alert_admin_receivers_rw, "method":"PATCH", "path":alerts_receivers_uuid_path, "project": "11111111-1111-1111-1111-111111111111"}

    # /edgenode/api/v1/alerts/receivers/<uuid>/template
    not allow_alrt_r with input as {"roles":alerts_admin_r, "method":"PATCH", "path":alerts_receivers_uuid_template_path, "project": "11111111-1111-1111-1111-111111111111"}
    not allow_alrt_rw with input as {"roles":alerts_admin_rw, "method":"PATCH", "path":alerts_receivers_uuid_template_path, "project": "11111111-1111-1111-1111-111111111111"}
    allow_alert_rx_rw with input as {"roles":alert_admin_receivers_rw, "method":"PATCH", "path":alerts_receivers_uuid_template_path, "project": "11111111-1111-1111-1111-111111111111"}
}

# disallow all policies for an unauthorized role
test_unauthorized_alerts_read if {
    some path in all_get_paths
    not allow_alrt_r with input as {"roles":unauthorized_role, "method":"GET", "path":path, "project": "11111111-1111-1111-1111-111111111111"}
    not allow_alrt_rw with input as {"roles":unauthorized_role, "method":"GET", "path":path, "project": "11111111-1111-1111-1111-111111111111"}
    not allow_alert_rx_rw with input as {"roles":unauthorized_role, "method":"GET", "path":path, "project": "11111111-1111-1111-1111-111111111111"}
}

test_unauthorized_alerts_patch if {
    some path in all_patch_paths
    not allow_alrt_r with input as {"roles":unauthorized_role, "method":"PATCH", "path":path, "project": "11111111-1111-1111-1111-111111111111"}
    not allow_alrt_rw with input as {"roles":unauthorized_role, "method":"PATCH", "path":path, "project": "11111111-1111-1111-1111-111111111111"}
    not allow_alert_rx_rw with input as {"roles":unauthorized_role, "method":"PATCH", "path":path, "project": "11111111-1111-1111-1111-111111111111"}
}
