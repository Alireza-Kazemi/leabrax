// Copyright (c) 2020, The Emergent Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package glong

import (
	"github.com/ccnlab/leabrax/leabra"
	"github.com/chewxy/math32"
	"github.com/goki/ki/kit"
)

///////////////////////////////////////////////////////////////////////////
// Layer

// Layer has GABA-B and NMDA channels, with longer time-constants,
// to supports bistable activation dynamics including active maintenance
// in frontal cortex.  NMDA requires NMDAPrjn on relevant projections.
// It also records AlphaMax = maximum activation within an AlphaCycle,
// which is important given the transient dynamics.
type Layer struct {
	leabra.Layer
	NMDA    NMDAParams  `view:"inline" desc:"NMDA channel parameters plus more general params"`
	GABAB   GABABParams `view:"inline" desc:"GABA-B / GIRK channel parameters"`
	GlNeurs []Neuron    `desc:"slice of extra glong.Neuron state for this layer -- flat list of len = Shape.Len(). You must iterate over index and use pointer to modify values."`
}

var KiT_Layer = kit.Types.AddType(&Layer{}, leabra.LayerProps)

func (ly *Layer) Defaults() {
	ly.Layer.Defaults()
	ly.NMDA.Defaults()
	ly.GABAB.Defaults()
	ly.Act.Init.Decay = 0
	ly.Act.Erev.K = .1 // -90mv
}

func (ly *Layer) InitGlong() {
	for ni := range ly.GlNeurs {
		nrn := &ly.GlNeurs[ni]
		nrn.AlphaMax = 0
		nrn.VmEff = 0
		nrn.Gnmda = 0
		nrn.NMDA = 0
		nrn.NMDASyn = 0
		nrn.GgabaB = 0
		nrn.GABAB = 0
		nrn.GABABx = 0
	}
}

// InitAlphaMax initializes the AlphaMax to 0
func (ly *Layer) InitAlphaMax() {
	for ni := range ly.GlNeurs {
		nrn := &ly.GlNeurs[ni]
		nrn.AlphaMax = 0
	}
}

func (ly *Layer) InitGInc() {
	ly.Layer.InitGInc()
	for ni := range ly.GlNeurs {
		nrn := &ly.GlNeurs[ni]
		nrn.NMDASyn = 0
	}
}

func (ly *Layer) InitActs() {
	ly.Layer.InitActs()
	ly.InitGlong()
}

func (ly *Layer) DecayState(decay float32) {
	ly.Layer.DecayState(decay)
	for ni := range ly.GlNeurs {
		gnr := &ly.GlNeurs[ni]
		gnr.VmEff -= decay * gnr.VmEff
		gnr.Gnmda -= decay * gnr.Gnmda
		gnr.NMDA -= decay * gnr.NMDA
		gnr.NMDASyn -= decay * gnr.NMDASyn
		gnr.GgabaB -= decay * gnr.GgabaB
		gnr.GABAB -= decay * gnr.GABAB
		gnr.GABABx -= decay * gnr.GABABx
	}
}

func (ly *Layer) AlphaCycInit() {
	ly.Layer.AlphaCycInit()
	ly.InitAlphaMax()
}

// GFmInc integrates new synaptic conductances from increments sent during last SendGDelta.
func (ly *Layer) GFmInc(ltime *leabra.Time) {
	ly.RecvGInc(ltime)
	ly.RecvGnmdaPInc(ltime)
	ly.GFmIncNeur(ltime)
}

// RecvGInc calls RecvGInc on receiving projections to collect Neuron-level G*Inc values.
// This is called by GFmInc overall method, but separated out for cases that need to
// do something different.
func (ly *Layer) RecvGInc(ltime *leabra.Time) {
	for _, p := range ly.RcvPrjns {
		if p.IsOff() {
			continue
		}
		if p.Type() == NMDA { // skip NMDA
			continue
		}
		p.(leabra.LeabraPrjn).RecvGInc()
	}
}

// RecvGnmdaPInc increments the recurrent-specific GeInc
func (ly *Layer) RecvGnmdaPInc(ltime *leabra.Time) {
	for _, p := range ly.RcvPrjns {
		if p.IsOff() {
			continue
		}
		if p.Type() != NMDA { // skip non-NMDA
			continue
		}
		pj := p.(leabra.LeabraPrjn).AsLeabra()
		for ri := range ly.GlNeurs {
			rn := &ly.GlNeurs[ri]
			rn.NMDASyn += pj.GInc[ri]
			pj.GInc[ri] = 0
		}
	}
}

// GFmIncNeur is the neuron-level code for GFmInc that integrates overall Ge, Gi values
// from their G*Raw accumulators.
func (ly *Layer) GFmIncNeur(ltime *leabra.Time) {
	for ni := range ly.Neurons {
		nrn := &ly.Neurons[ni]
		if nrn.IsOff() {
			continue
		}
		ly.Act.GiFmRaw(nrn, nrn.GiRaw)

		gnr := &ly.GlNeurs[ni]
		gnr.VmEff = ly.NMDA.VmEff(nrn.Vm, nrn.Act)

		gnr.NMDA = ly.NMDA.NMDA(gnr.NMDA, gnr.NMDASyn)
		gnr.Gnmda = ly.NMDA.Gnmda(gnr.NMDA, gnr.VmEff)

		ly.Act.GeFmRaw(nrn, nrn.GeRaw+gnr.Gnmda)
	}
}

func (ly *Layer) GABABFmGi(ltime *leabra.Time) {
	for ni := range ly.Neurons {
		nrn := &ly.Neurons[ni]
		if nrn.IsOff() {
			continue
		}
		gnr := &ly.GlNeurs[ni]
		gnr.GABAB, gnr.GABABx = ly.GABAB.GABAB(gnr.GABAB, gnr.GABABx, nrn.Gi)
		gnr.GgabaB = ly.GABAB.GgabaB(gnr.GABAB, gnr.VmEff)
		if ly.Act.KNa.On {
			nrn.Gk += gnr.GgabaB // Gk was set by KNa
		} else {
			nrn.Gk = gnr.GgabaB
		}
	}
}

func (ly *Layer) ActFmG(ltime *leabra.Time) {
	ly.Layer.ActFmG(ltime)
	ly.GABABFmGi(ltime)
	if ltime.Cycle >= ly.NMDA.AlphaMaxCyc {
		ly.AlphaMaxFmAct(ltime)
	}
}

// AlphaMaxFmAct computes AlphaMax from Activation
func (ly *Layer) AlphaMaxFmAct(ltime *leabra.Time) {
	for ni := range ly.Neurons {
		nrn := &ly.Neurons[ni]
		if nrn.IsOff() {
			continue
		}
		gnr := &ly.GlNeurs[ni]
		gnr.AlphaMax = math32.Max(gnr.AlphaMax, nrn.Act)
	}
}

// ActLrnFmAlphaMax sets ActLrn to AlphaMax
func (ly *Layer) ActLrnFmAlphaMax() {
	for ni := range ly.Neurons {
		nrn := &ly.Neurons[ni]
		if nrn.IsOff() {
			continue
		}
		gnr := &ly.GlNeurs[ni]
		nrn.ActLrn = gnr.AlphaMax
	}
}

// MaxAlphaMax returns the maximum AlphaMax across the layer
func (ly *Layer) MaxAlphaMax() float32 {
	mx := float32(0)
	for ni := range ly.Neurons {
		nrn := &ly.Neurons[ni]
		if nrn.IsOff() {
			continue
		}
		gnr := &ly.GlNeurs[ni]
		mx = math32.Max(gnr.AlphaMax, mx)
	}
	return mx
}

///////////////////////////////////////////////////////////////////////////
// Neurons

// Build constructs the layer state, including calling Build on the projections.
func (ly *Layer) Build() error {
	err := ly.Layer.Build()
	if err != nil {
		return err
	}
	ly.GlNeurs = make([]Neuron, len(ly.Neurons))
	return nil
}

// UnitVarIdx returns the index of given variable within the Neuron,
// according to UnitVarNames() list (using a map to lookup index),
// or -1 and error message if not found.
func (ly *Layer) UnitVarIdx(varNm string) (int, error) {
	vidx, err := ly.Layer.UnitVarIdx(varNm)
	if err == nil {
		return vidx, err
	}
	vidx, err = NeuronVarIdxByName(varNm)
	if err != nil {
		return -1, err
	}
	nn := ly.Layer.UnitVarNum()
	return nn + vidx, nil
}

// UnitVal1D returns value of given variable index on given unit, using 1-dimensional index.
// returns NaN on invalid index.
// This is the core unit var access method used by other methods,
// so it is the only one that needs to be updated for derived layer types.
func (ly *Layer) UnitVal1D(varIdx int, idx int) float32 {
	if varIdx < 0 {
		return math32.NaN()
	}
	nn := ly.Layer.UnitVarNum()
	if varIdx < nn {
		return ly.Layer.UnitVal1D(varIdx, idx)
	}
	if idx < 0 || idx >= len(ly.Neurons) {
		return math32.NaN()
	}
	varIdx -= nn
	if varIdx > len(NeuronVars) {
		return math32.NaN()
	}
	gnr := &ly.GlNeurs[idx]
	return gnr.VarByIndex(varIdx)
}

// UnitVarNum returns the number of Neuron-level variables
// for this layer.  This is needed for extending indexes in derived types.
func (ly *Layer) UnitVarNum() int {
	return ly.Layer.UnitVarNum() + len(NeuronVars)
}
