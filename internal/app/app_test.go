package app
import "testing"
func TestTransitions(t *testing.T){valid:=[][2]string{{"RECEIVED","WAITING_FOR_DRYING"},{"WAITING_FOR_DRYING","DRYING"},{"DRYING","DRYING_COMPLETED"}};for _,x:=range valid{if !validTransition(x[0],x[1]){t.Fatalf("expected valid %v",x)}};if validTransition("COMPLETED","DRYING"){t.Fatal("completed job must not restart")}}
