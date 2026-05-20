package novaluereceivers

type Controller struct{}

func (c Controller) Reconcile() {} // want "method receiver must be pointer receiver"

func (c *Controller) Observe() {}

//projectlint:allow value-receiver documented immutable example
func (c Controller) AllowedValueReceiver() {}
