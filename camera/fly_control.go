// Copyright 2016 The G3N Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package camera

import (
	"github.com/g3n/engine/core"
	"github.com/g3n/engine/gui"
	"github.com/g3n/engine/math32"
	"github.com/g3n/engine/window"
)

// qrot creates a quaterion from an angle in radians and an axis of rotation.
func qrot(angle float32, axis *math32.Vector3) *math32.Quaternion {
	// https://en.wikipedia.org/wiki/Quaternions_and_spatial_rotation
	c, s := math32.Cos(angle/2.0), math32.Sin(angle/2.0)
	x := axis.X * s
	y := axis.Y * s
	z := axis.Z * s

	return math32.NewQuaternion(x, y, z, c)
}

// fly camera allows user to move camera in "FPS" or "flight sim" style
// movement.
// - translation along 3 axes: forward/backward, up/down, left/right (strafe)
// - rotation about 3 axes: roll, pitch, yaw
// thus requires at least 12 inputs.
// will also allow "zoom" (change fov), so +2 inputs.
//
// options
// - clamp pitch to -90/+90 deg from horizontal (prevent loop-de-loops)
// - look-spring - returns pitch to 0 deg when moving forward/backward
// - mouse-look - captures cursor and makes mouse movements into pitch and yaw.
// - invert pitch - inverts control of pitch so that normal "up" pitched "down", viceversa
// - movements speeds
// - buttons/etc
//
// internally, "right" will be +X, "up" will be +Y, and "forward" will be -Z
// a right handed coordinate system mirroring OpenGL's camera conventions.

type FlyMovement int

const (
	Forward FlyMovement = iota
	Backward
	Right
	Left
	Up
	Down
	PitchUp
	PitchDown
	YawRight
	YawLeft
	RollRight
	RollLeft
	ZoomIn
	ZoomOut
	maxmovement = ZoomOut
)

const degrees = math32.Pi / 180.0
const radians = 1.0 / degrees

type FlyControl struct {
	core.Dispatcher         // Embedded event dispatcher
	cam             *Camera // Controlled camera

	position *math32.Vector3 // camera position in the world
	rotation *math32.Vector3 // camera yaw, pitch, and roll (radians)
	forward  *math32.Vector3 // forward direction of camera
	up       *math32.Vector3 // up direction of camera
	upWorld  *math32.Vector3 // The world up direction

	// Constraints map.
	// translation movements aren't used.
	// angular contraints are in radians.
	Constraints map[FlyMovement]float32

	// Speeds map.
	// translation movements in units/event,
	// angular movement and zoom in radians/event.
	Speeds map[FlyMovement]float32

	// Keys map.
	Keys map[FlyMovement]window.Key
}

func NewFlyControl() *FlyControl {

	fc := new(FlyControl)

	// Subscribe to events
	// copied from orbit_control.go
	// gui.Manager().SubscribeID(window.OnMouseUp, &fc, fc.onMouse)
	// gui.Manager().SubscribeID(window.OnMouseDown, &fc, fc.onMouse)
	// gui.Manager().SubscribeID(window.OnScroll, &fc, fc.onScroll)
	gui.Manager().SubscribeID(window.OnKeyDown, &fc, fc.onKey)
	gui.Manager().SubscribeID(window.OnKeyRepeat, &fc, fc.onKey)
	// gui.Manager().SubscribeID(window.OnKeyUp, &fc, fc.onKeyUp)
	// fc.SubscribeID(window.OnCursor, &fc, fc.onCursor)

	return fc
}

// Dispose unsubscribes from all events.
func (fc *FlyControl) Dispose() {
	// copied from orbit_control.go
	gui.Manager().UnsubscribeID(window.OnMouseUp, &fc)
	gui.Manager().UnsubscribeID(window.OnMouseDown, &fc)
	gui.Manager().UnsubscribeID(window.OnScroll, &fc)
	gui.Manager().UnsubscribeID(window.OnKeyDown, &fc)
	gui.Manager().UnsubscribeID(window.OnKeyRepeat, &fc)
	gui.Manager().UnsubscribeID(window.OnKeyUp, &fc)
	fc.UnsubscribeID(window.OnCursor, &fc)
}

func (fc *FlyControl) Reposition(position *math32.Vector3) {
	fc.position.Copy(position)
	fc.cam.SetPositionVec(position)
}

func (fc *FlyControl) Reorient(target, worldUp *math32.Vector3) {
	fc.forward = target.Clone().Sub(fc.position).Normalize()
	right := fc.forward.Clone().Cross(worldUp).Normalize()
	fc.up = right.Cross(fc.forward).Normalize()
}

// movement changes

func (fc *FlyControl) Forward(delta float32) {
	deltaDir := fc.forward.Clone().MultiplyScalar(delta)
	fc.position.Add(deltaDir)
	fc.cam.SetPositionVec(fc.position)
}

func (fc *FlyControl) Right(delta float32) {
	// right direction from forward cross up
	deltaDir := fc.forward.Clone().Cross(fc.up).MultiplyScalar(delta)
	fc.position.Add(deltaDir)
	fc.cam.SetPositionVec(fc.position)
}

func (fc *FlyControl) Up(delta float32) {
	deltaDir := fc.up.Clone().MultiplyScalar(delta)
	fc.position.Add(deltaDir)
	fc.cam.SetPositionVec(fc.position)
}

func (fc *FlyControl) Yaw(delta float32) {
	yaw := fc.rotation.X + delta
	if fc.constraintOk(yaw, YawLeft, YawRight) {
		fc.rotation.X = yaw
		// rotation about up axis
		fc.forward.ApplyAxisAngle(fc.up, delta)
		fc.cam.RotateOnAxis(fc.up, delta)
	}
}

func (fc *FlyControl) Pitch(delta float32) {
	pitch := fc.rotation.Y + delta
	if fc.constraintOk(pitch, PitchDown, PitchUp) {
		fc.rotation.Y = delta
		// rotation about right axis
		// affects both "forward" and "up" directions
		right := fc.forward.Clone().Cross(fc.up)
		fc.forward.ApplyAxisAngle(right, delta)
		fc.up.ApplyAxisAngle(right, delta)
		fc.cam.RotateOnAxis(right, delta)
	}
}

func (fc *FlyControl) Roll(delta float32) {
	roll := fc.rotation.Z + delta
	if fc.constraintOk(roll, RollLeft, RollRight) {
		fc.rotation.Z = roll
		// rotation about forward axis
		fc.up.ApplyAxisAngle(fc.forward, delta)
		fc.cam.RotateOnAxis(fc.forward, delta)
	}
}

func (fc *FlyControl) Zoom(delta float32) {
	newFovRad := fc.cam.Fov()*degrees + delta
	if fc.constraintOk(newFovRad, ZoomIn, ZoomOut) {
		fc.cam.SetFov(newFovRad * radians)
	}
}

func (fc *FlyControl) ScaleZoom(fovScale float32) {
	newFovRad := (fc.cam.Fov() * degrees) * fovScale
	if fc.constraintOk(newFovRad, ZoomIn, ZoomOut) {
		fc.cam.SetFov(newFovRad * radians)
	}
}

// checks if newValue is allowed according to the given low and high
// movement constraints.
func (fc *FlyControl) constraintOk(newValue float32, low, high FlyMovement) bool {
	min, minDefined := fc.Constraints[low]
	max, maxDefined := fc.Constraints[high]
	if minDefined && newValue < min {
		return false
	}
	if maxDefined && newValue > max {
		return false
	}
	return true
}

func (fc *FlyControl) apply(movement FlyMovement, delta float32) {
	switch movement {
	case Backward, Forward:
		fc.Forward(delta)
	case Left, Right:
		fc.Right(delta)
	case Up, Down:
		fc.Up(delta)
	case YawLeft, YawRight:
		fc.Yaw(delta)
	case PitchUp, PitchDown:
		fc.Pitch(delta)
	case RollLeft, RollRight:
		fc.Roll(delta)
	case ZoomIn, ZoomOut:
		fc.Zoom(delta)
	}
}

// copied from orbit_control.go

// onMouse is called when an OnMouseDown/OnMouseUp event is received.
/*func (fc *FlyControl) onMouse(evname string, ev interface{}) {

	// If nothing enabled ignore event
	if fc.enabled == OrbitNone {
		return
	}

	switch evname {
	case window.OnMouseDown:
		gui.Manager().SetCursorFocus(fc)
		mev := ev.(*window.MouseEvent)
		switch mev.Button {
		case window.MouseButtonLeft: // Rotate
			if fc.enabled&OrbitRot != 0 {
				fc.state = stateRotate
				fc.rotStart.Set(mev.Xpos, mev.Ypos)
			}
		case window.MouseButtonMiddle: // Zoom
			if fc.enabled&OrbitZoom != 0 {
				fc.state = stateZoom
				fc.zoomStart = mev.Ypos
			}
		case window.MouseButtonRight: // Pan
			if fc.enabled&OrbitPan != 0 {
				fc.state = statePan
				fc.panStart.Set(mev.Xpos, mev.Ypos)
			}
		}
	case window.OnMouseUp:
		gui.Manager().SetCursorFocus(nil)
		fc.state = stateNone
	}
}*/

// onCursor is called when an OnCursor event is received.
/*func (fc *FlyControl) onCursor(evname string, ev interface{}) {

	// If nothing enabled ignore event
	if fc.enabled == OrbitNone || fc.state == stateNone {
		return
	}

	mev := ev.(*window.CursorEvent)
	switch fc.state {
	case stateRotate:
		c := -2 * math32.Pi * fc.RotSpeed / fc.winSize()
		fc.Rotate(c*(mev.Xpos-fc.rotStart.X),
			c*(mev.Ypos-fc.rotStart.Y))
		fc.rotStart.Set(mev.Xpos, mev.Ypos)
	case stateZoom:
		fc.Zoom(fc.ZoomSpeed * (mev.Ypos - fc.zoomStart))
		fc.zoomStart = mev.Ypos
	case statePan:
		fc.Pan(mev.Xpos-fc.panStart.X,
			mev.Ypos-fc.panStart.Y)
		fc.panStart.Set(mev.Xpos, mev.Ypos)
	}
}*/

// onScroll is called when an OnScroll event is received.
/*func (fc *FlyControl) onScroll(evname string, ev interface{}) {

	if fc.enabled&OrbitZoom != 0 {
		sev := ev.(*window.ScrollEvent)
		fc.Zoom(-sev.Yoffset)
	}
}*/

// onKey is called when an OnKeyDown/OnKeyRepeat event is received.
func (fc *FlyControl) onKey(evname string, ev interface{}) {

	// If keyboard control is disabled ignore event
	// if fc.enabled&OrbitKeys == 0 {
	// 	return
	// }

	kev := ev.(*window.KeyEvent)
	// find which movement the key corresponds to
	var movement FlyMovement = -1
	for m, k := range fc.Keys {
		if k == kev.Key {
			movement = m
			break
		}
	}
	if movement == -1 {
		// the pressed key is not mapped to a camera movement
		return
	}

	delta := fc.Speeds[movement]
	fc.apply(movement, delta)
}

// onKeyUp is called when an OnKeyUp event is received.
// func (fc *FlyControl) onKeyUp(evname string, ev interface{}) {
// }

// winSize returns the window height or width based on the camera reference axis.
func (fc *FlyControl) winSize() float32 {

	width, size := window.Get().GetSize()
	if fc.cam.Axis() == Horizontal {
		size = width
	}
	return float32(size)
}
