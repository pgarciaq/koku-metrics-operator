package collector

import "testing"

func TestInstanceTypeFromNodeLabels(t *testing.T) {
	tests := []struct {
		name       string
		nodeLabels string
		want       string
	}{
		{
			name: "node.kubernetes.io instance type",
			nodeLabels: "label_beta_kubernetes_io_arch:amd64|" +
				"label_node_kubernetes_io_instance_type:m5.2xlarge|" +
				"label_kubernetes_io_os:linux",
			want: "m5.2xlarge",
		},
		{
			name: "beta.kubernetes.io fallback",
			nodeLabels: "label_beta_kubernetes_io_instance_type:m5.xlarge|" +
				"label_kubernetes_io_os:linux",
			want: "m5.xlarge",
		},
		{
			name:       "empty labels",
			nodeLabels: "",
			want:       "",
		},
		{
			name:       "no instance type label",
			nodeLabels: "label_kubernetes_io_os:linux|label_arch:amd64",
			want:       "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := instanceTypeFromNodeLabels(tt.nodeLabels)
			if got != tt.want {
				t.Errorf("instanceTypeFromNodeLabels() = %q, want %q", got, tt.want)
			}
		})
	}
}
